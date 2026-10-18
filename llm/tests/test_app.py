"""Prompt building and the answer schema. No model needed."""

import json

import jinja2
import pytest
from fastapi.testclient import TestClient

import app

CTX = app.Context(
    grammar="Word order is SVO.",
    morphology="verb: ...",
    entries=[
        app.Entry(id="see", word="narar", pos="verb", gloss="see", definition="To see."),
        app.Entry(id="woman", word="mena", pos="noun", gender="f", gloss="woman"),
    ],
    examples=[app.Example(english="The woman sees.", rifatian="Lu mena nara.", gloss="the woman see-TH.3SG")],
)

ANSWER = app.Answer(
    translation="Lu mena nara.",
    words=[app.AnswerWord(text="nara", lexicon_id="see", features=["PRS", "3SG"])],
    notes="",
)


def test_system_prompt_has_every_part():
    s = app.system_prompt(CTX)
    assert "Word order is SVO." in s
    assert "verb: ..." in s
    assert "Rifatian: Lu mena nara." in s
    assert "see | narar | verb | - | see | To see." in s
    assert "woman | mena | noun | f | woman | " in s


def test_system_prompt_order_keeps_stable_prefix_first():
    # The dictionary changes per sentence in "retrieved" mode; it must come last.
    s = app.system_prompt(CTX)
    assert s.index("# GRAMMAR") < s.index("# EXAMPLES") < s.index("# DICTIONARY")


def test_messages_alternate_after_feedback():
    req = app.TranslateRequest(
        english="The woman sees.",
        context=CTX,
        history=[app.Turn(answer=ANSWER, feedback="nara: wrong")],
    )
    msgs = app.build_messages(req)
    assert [m["role"] for m in msgs] == ["system", "user", "assistant", "user"]
    assert json.loads(msgs[2]["content"])["translation"] == "Lu mena nara."
    assert "nara: wrong" in msgs[3]["content"]
    assert "translate again: The woman sees." in msgs[3]["content"]


def test_schema_requires_every_field():
    schema = app.Answer.model_json_schema()
    assert set(schema["required"]) == {"translation", "words", "notes"}


TOOLS = [app.Tool(name="inflect", description="Inflect.", parameters={"type": "object", "properties": {}})]
STEP = app.Step(
    calls=[app.ToolCall(name="inflect", arguments={"id": "see", "features": ["PRS", "3SG"]})],
    results=[app.ToolResult(name="inflect", content="nara  (see-TH.3SG)")],
)


def test_system_prompt_tools_section():
    assert "# TOOLS" not in app.system_prompt(CTX)
    s = app.system_prompt(CTX, TOOLS)
    assert "# TOOLS" in s and "inflect:" in s
    none = app.Context(grammar="g", entries=[])
    assert "use the lookup tool" in app.system_prompt(none, TOOLS)


def test_messages_with_tool_steps():
    req = app.TranslateRequest(
        english="The woman sees.",
        context=CTX,
        tools=TOOLS,
        history=[app.Turn(steps=[STEP], answer=ANSWER, feedback="wrong")],
        steps=[STEP],
        final=True,
    )
    msgs = app.build_messages(req)
    roles = [m["role"] for m in msgs]
    assert roles == ["system", "user", "assistant", "tool", "assistant", "user", "assistant", "tool", "user"]
    assert msgs[2]["tool_calls"][0]["function"] == {"name": "inflect", "arguments": {"id": "see", "features": ["PRS", "3SG"]}}
    assert msgs[3] == {"role": "tool", "content": "nara  (see-TH.3SG)", "tool_name": "inflect"}
    assert "final translation" in msgs[-1]["content"]


class FakeCall:
    def __init__(self, name, arguments):
        self.function = type("F", (), {"name": name, "arguments": arguments})()


class FakeResp:
    def __init__(self, content, calls=None):
        self.message = type("M", (), {"content": content, "tool_calls": calls})()
        self.prompt_eval_count = 100
        self.eval_count = 20


class FakeClient:
    def __init__(self, content, calls=None):
        self.content = content
        self.calls = calls
        self.kwargs = None

    def chat(self, **kwargs):
        self.kwargs = kwargs
        return FakeResp(self.content, self.calls)


@pytest.mark.parametrize(
    "content,status",
    [(ANSWER.model_dump_json(), 200), ("not json", 502), ('{"translation": "x"}', 502)],
)
def test_translate_endpoint(monkeypatch, content, status):
    fake = FakeClient(content)
    monkeypatch.setattr(app, "client", lambda: fake)
    r = TestClient(app.app).post(
        "/translate", json={"english": "The woman sees.", "context": CTX.model_dump()}
    )
    assert r.status_code == status, r.text
    assert fake.kwargs["format"] == app.Answer.model_json_schema()
    assert fake.kwargs["options"]["temperature"] == 0.0
    if status == 200:
        assert r.json()["answer"]["translation"] == "Lu mena nara."
        assert r.json()["prompt_tokens"] == 100


class RaisingClient:
    def __init__(self, exc):
        self.exc = exc

    def chat(self, **kwargs):
        raise self.exc


@pytest.mark.parametrize(
    "exc,status,says",
    [
        (ConnectionError("Failed to connect"), 503, "ollama not reachable"),
        (RuntimeError("boom"), 502, "ollama call failed: RuntimeError: boom"),
    ],
)
def test_translate_ollama_errors(monkeypatch, exc, status, says):
    r = post(monkeypatch, RaisingClient(exc))
    assert r.status_code == status, r.text
    assert says in r.json()["detail"]


def test_template_error_is_not_blamed_on_ollama(monkeypatch):
    def broken(_):
        raise jinja2.UndefinedError("'tools' is undefined")

    monkeypatch.setattr(app, "build_messages", broken)
    monkeypatch.setattr(app, "client", lambda: FakeClient("{}"))
    r = TestClient(app.app, raise_server_exceptions=False).post(
        "/translate", json={"english": "The woman sees.", "context": CTX.model_dump()}
    )
    assert r.status_code == 500
    assert "ollama" not in r.text


def post(monkeypatch, fake, **extra):
    monkeypatch.setattr(app, "client", lambda: fake)
    body = {"english": "The woman sees.", "context": CTX.model_dump(), **extra}
    return TestClient(app.app).post("/translate", json=body)


def test_tool_round_returns_calls(monkeypatch):
    fake = FakeClient("", [FakeCall("inflect", {"id": "see", "features": ["PRS", "3SG"]})])
    r = post(monkeypatch, fake, tools=[t.model_dump() for t in TOOLS])
    assert r.status_code == 200, r.text
    assert "format" not in fake.kwargs
    assert fake.kwargs["options"]["num_predict"] == app.TOOL_ROUND_MAX_TOKENS
    assert fake.kwargs["tools"][0]["function"]["name"] == "inflect"
    assert r.json()["calls"] == [{"name": "inflect", "arguments": {"id": "see", "features": ["PRS", "3SG"]}}]
    assert r.json()["answer"] is None


def test_tool_round_without_calls_returns_content(monkeypatch):
    r = post(monkeypatch, FakeClient("done"), tools=[t.model_dump() for t in TOOLS])
    assert r.status_code == 200, r.text
    assert r.json()["calls"] == [] and r.json()["content"] == "done"


def test_final_round_uses_format(monkeypatch):
    fake = FakeClient(ANSWER.model_dump_json())
    r = post(monkeypatch, fake, tools=[t.model_dump() for t in TOOLS], final=True)
    assert r.status_code == 200, r.text
    assert "tools" not in fake.kwargs and fake.kwargs["format"] == app.Answer.model_json_schema()
    assert r.json()["answer"]["translation"] == "Lu mena nara."
