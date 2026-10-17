"""Local LLM service for lingua-nova.

One request = one translation attempt. The Go side (internal/llm) owns
the retry loop and the validator; this service only turns a request into
a prompt, asks the local model (Ollama) for an answer constrained to a
JSON schema, and checks that the answer has that shape.

Run:  ollama serve   (in another terminal)
      uv run uvicorn app:app --port 8765
"""

import json
import os
import time
from pathlib import Path

import ollama
from fastapi import FastAPI, HTTPException
from jinja2 import Environment, FileSystemLoader, StrictUndefined
from pydantic import BaseModel, Field, ValidationError

OLLAMA_HOST = os.environ.get("OLLAMA_HOST", "http://127.0.0.1:11434")
DEFAULT_MODEL = "qwen2.5:7b-instruct-q4_K_M"

templates = Environment(
    loader=FileSystemLoader(Path(__file__).parent / "prompts"),
    undefined=StrictUndefined,
    keep_trailing_newline=True,
)


# --- The answer the model must produce (also the JSON schema sent to Ollama).


class AnswerWord(BaseModel):
    text: str = Field(description="The word as it appears in the translation, without punctuation.")
    lexicon_id: str = Field(description="The id of the dictionary entry this word is a form of.")
    features: list[str] = Field(description="Inflection features, e.g. [\"PRS\", \"3SG\"] or [\"PL\"]; [] if none.")


class Answer(BaseModel):
    translation: str = Field(description="The Rifatian sentence, with capital letter and final punctuation.")
    words: list[AnswerWord] = Field(description="One item per word of the translation, in order.")
    notes: str = Field(description="Anything uncertain, e.g. a missing word. Empty if none.")


# --- The request from Go.


class Entry(BaseModel):
    id: str
    word: str  # the modern form
    pos: str
    gender: str = ""
    gloss: str
    definition: str = ""


class Example(BaseModel):
    english: str
    rifatian: str
    gloss: str


class Context(BaseModel):
    grammar: str
    morphology: str = ""
    entries: list[Entry]
    examples: list[Example] = []


class Turn(BaseModel):
    """A failed attempt and the validator's feedback on it."""

    answer: Answer
    feedback: str


class TranslateRequest(BaseModel):
    english: str
    model: str = DEFAULT_MODEL
    context: Context
    history: list[Turn] = []
    num_ctx: int = 8192
    temperature: float = 0.0


class TranslateResponse(BaseModel):
    answer: Answer
    model: str
    prompt_tokens: int
    eval_tokens: int
    duration_ms: int


# --- Prompt building (pure functions, tested without a model).


def system_prompt(ctx: Context) -> str:
    """The system prompt. Stable parts first (rules, grammar, examples),
    then the dictionary, so consecutive requests share a long prefix and
    Ollama can reuse its KV cache for it."""
    return templates.get_template("system.j2").render(ctx=ctx)


def user_prompt(english: str) -> str:
    return templates.get_template("user.j2").render(english=english)


def build_messages(req: TranslateRequest) -> list[dict]:
    """system, user, then (assistant answer, user feedback) per failed attempt."""
    msgs = [
        {"role": "system", "content": system_prompt(req.context)},
        {"role": "user", "content": user_prompt(req.english)},
    ]
    for turn in req.history:
        msgs.append({"role": "assistant", "content": turn.answer.model_dump_json()})
        msgs.append(
            {
                "role": "user",
                "content": templates.get_template("feedback.j2").render(
                    feedback=turn.feedback, english=req.english
                ),
            }
        )
    return msgs


# --- HTTP.

app = FastAPI(title="lingua-nova llm")


def client() -> ollama.Client:
    return ollama.Client(host=OLLAMA_HOST)


@app.get("/health")
def health(model: str = DEFAULT_MODEL) -> dict:
    try:
        names = [m.model for m in client().list().models]
    except Exception as e:  # connection refused, etc.
        raise HTTPException(503, f"ollama not reachable at {OLLAMA_HOST}: {e}. Start it with: ollama serve")
    if model not in names:
        raise HTTPException(503, f"model {model} not pulled; have {names}. Run: ollama pull {model}")
    return {"ok": True, "ollama": OLLAMA_HOST, "model": model}


@app.post("/translate")
def translate(req: TranslateRequest) -> TranslateResponse:
    start = time.monotonic()
    try:
        resp = client().chat(
            model=req.model,
            messages=build_messages(req),
            format=Answer.model_json_schema(),
            options={"temperature": req.temperature, "num_ctx": req.num_ctx},
        )
    except ollama.ResponseError as e:
        raise HTTPException(502, f"ollama: {e.error}")
    except Exception as e:
        raise HTTPException(503, f"ollama not reachable at {OLLAMA_HOST}: {e}. Start it with: ollama serve")

    raw = resp.message.content or ""
    try:
        answer = Answer.model_validate(json.loads(raw))
    except (json.JSONDecodeError, ValidationError) as e:
        raise HTTPException(502, f"model reply is not a valid answer: {e}\nraw: {raw[:500]}")

    return TranslateResponse(
        answer=answer,
        model=req.model,
        prompt_tokens=resp.prompt_eval_count or 0,
        eval_tokens=resp.eval_count or 0,
        duration_ms=int((time.monotonic() - start) * 1000),
    )
