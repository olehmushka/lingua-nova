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
    assert "seed" not in fake.kwargs["options"]
    if status == 200:
        assert r.json()["answer"]["translation"] == "Lu mena nara."
        assert r.json()["prompt_tokens"] == 100


def test_translate_seed(monkeypatch):
    fake = FakeClient(ANSWER.model_dump_json())
    monkeypatch.setattr(app, "client", lambda: fake)
    r = TestClient(app.app).post(
        "/translate",
        json={"english": "The woman sees.", "context": CTX.model_dump(), "temperature": 0.7, "seed": 3},
    )
    assert r.status_code == 200, r.text
    assert fake.kwargs["options"]["temperature"] == 0.7
    assert fake.kwargs["options"]["seed"] == 3


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


# --- /coin

CONCEPT = app.Concept(english="owl", pos="noun", gender="f")
COINAGE = app.Coinage(
    definition="A bird of prey that hunts at night.",
    candidates=[app.Candidate(proto="hurula", reason="Soft sounds, like its call.")],
)


def coin_req(**extra):
    return app.CoinRequest(
        concept=CONCEPT,
        phonology="Consonants: p t c",
        examples=[app.Root(proto="nauca", modern="nauga", pos="noun", gloss="fish")],
        **extra,
    )


def test_coin_prompt():
    msgs = app.coin_messages(coin_req(n=4))
    assert [m["role"] for m in msgs] == ["system", "user"]
    assert "Consonants: p t c" in msgs[0]["content"]
    assert "*nauca → nauga | noun | fish" in msgs[0]["content"]
    assert 'Coin 4 different proto roots for a new noun (feminine) meaning "owl".' in msgs[1]["content"]
    assert "typically end in -a" in msgs[1]["content"]
    assert "really different" in msgs[1]["content"] and 'what "owl" means' in msgs[1]["content"]
    verb = app.coin_messages(app.CoinRequest(concept=app.Concept(english="swim", pos="verb"), phonology="p"))
    assert "-ar, -er or -ir" in verb[1]["content"]
    assert "# EXAMPLES" not in verb[0]["content"]


def test_coin_messages_after_feedback():
    msgs = app.coin_messages(coin_req(history=[app.Round(coinage=COINAGE, feedback="*hurula: rejected")]))
    assert [m["role"] for m in msgs] == ["system", "user", "assistant", "user"]
    assert json.loads(msgs[2]["content"])["candidates"][0]["proto"] == "hurula"
    assert "*hurula: rejected" in msgs[3]["content"]
    assert 'new proto roots for "owl"' in msgs[3]["content"]


def test_coinage_schema_requires_every_field():
    schema = app.Coinage.model_json_schema()
    assert set(schema["required"]) == {"definition", "candidates"}
    assert set(schema["$defs"]["Candidate"]["required"]) == {"proto", "reason"}


@pytest.mark.parametrize(
    "content,status",
    [(COINAGE.model_dump_json(), 200), ("not json", 502), ('{"definition": "x"}', 502)],
)
def test_coin_endpoint(monkeypatch, content, status):
    fake = FakeClient(content)
    monkeypatch.setattr(app, "client", lambda: fake)
    body = coin_req(temperature=0.7, seed=2).model_dump()
    r = TestClient(app.app).post("/coin", json=body)
    assert r.status_code == status, r.text
    assert fake.kwargs["format"] == app.Coinage.model_json_schema()
    assert fake.kwargs["options"]["temperature"] == 0.7 and fake.kwargs["options"]["seed"] == 2
    if status == 200:
        assert r.json()["coinage"]["candidates"][0]["proto"] == "hurula"
        assert r.json()["prompt_tokens"] == 100


def test_coin_ollama_down(monkeypatch):
    monkeypatch.setattr(app, "client", lambda: RaisingClient(ConnectionError("Failed to connect")))
    r = TestClient(app.app).post("/coin", json=coin_req().model_dump())
    assert r.status_code == 503 and "ollama not reachable" in r.json()["detail"]


@pytest.mark.parametrize(
    "model,kind,want",
    [
        ("nomic-embed-text", "query", ["search_query: sea"]),
        ("nomic-embed-text:latest", "document", ["search_document: sea"]),
        ("other-embedder", "query", ["sea"]),
    ],
)
def test_embed_inputs_prefix(model, kind, want):
    assert app.embed_inputs(app.EmbedRequest(texts=["sea"], kind=kind, model=model)) == want


class FakeEmbedClient:
    def __init__(self, n=None):
        self.n = n  # vectors to return; None = one per input
        self.kwargs = None

    def embed(self, **kwargs):
        self.kwargs = kwargs
        n = len(kwargs["input"]) if self.n is None else self.n

        class Resp:
            embeddings = [[float(i), 1.0] for i in range(n)]

        return Resp()


@pytest.mark.parametrize("n,status", [(None, 200), (1, 502)])
def test_embed_endpoint(monkeypatch, n, status):
    fake = FakeEmbedClient(n)
    monkeypatch.setattr(app, "client", lambda: fake)
    r = TestClient(app.app).post("/embed", json={"texts": ["sea", "river (noun): A river."], "kind": "document"})
    assert r.status_code == status, r.text
    assert fake.kwargs["model"] == "nomic-embed-text"
    assert fake.kwargs["input"][1] == "search_document: river (noun): A river."
    if status == 200:
        assert r.json()["vectors"] == [[0.0, 1.0], [1.0, 1.0]]


def test_embed_empty_needs_no_model(monkeypatch):
    monkeypatch.setattr(app, "client", lambda: RaisingClient(ConnectionError("down")))
    r = TestClient(app.app).post("/embed", json={"texts": [], "kind": "query"})
    assert r.status_code == 200 and r.json()["vectors"] == []


def test_embed_ollama_down(monkeypatch):
    class Down:
        def embed(self, **kwargs):
            raise ConnectionError("Failed to connect")

    monkeypatch.setattr(app, "client", lambda: Down())
    r = TestClient(app.app).post("/embed", json={"texts": ["sea"], "kind": "query"})
    assert r.status_code == 503 and "ollama not reachable" in r.json()["detail"]
