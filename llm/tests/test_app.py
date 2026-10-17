"""Prompt building and the answer schema. No model needed."""

import json

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


class FakeResp:
    def __init__(self, content):
        self.message = type("M", (), {"content": content})()
        self.prompt_eval_count = 100
        self.eval_count = 20


class FakeClient:
    def __init__(self, content):
        self.content = content
        self.kwargs = None

    def chat(self, **kwargs):
        self.kwargs = kwargs
        return FakeResp(self.content)


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
