"""Local LLM service for lingua-nova.

One request = one model call. The Go side (internal/llm) owns the retry
loop, the tool loop, the tools and the validator; this service only turns a
request into a prompt and asks the local model (Ollama) either for tool
calls or for an answer constrained to a JSON schema, and checks that the
answer has that shape. /translate translates a sentence; /coin proposes
proto roots for a new word.

Run:  ollama serve   (in another terminal)
      uv run uvicorn app:app --port 8765
"""

import json
import logging
import os
import time
from pathlib import Path

import ollama
from fastapi import FastAPI, HTTPException
from jinja2 import Environment, FileSystemLoader, StrictUndefined
from pydantic import BaseModel, Field, ValidationError

OLLAMA_HOST = os.environ.get("OLLAMA_HOST", "http://127.0.0.1:11434")
DEFAULT_MODEL = "qwen2.5:7b-instruct-q4_K_M"

# Output caps in tokens. A tool round has no schema to end the reply, and a
# small model can ramble until the context window is full (minutes on a
# partly-CPU model); a few tool calls fit in 512, an answer in 1024.
TOOL_ROUND_MAX_TOKENS = 512
ANSWER_MAX_TOKENS = 1024

log = logging.getLogger("uvicorn.error")

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


class Tool(BaseModel):
    """A function the model may call. Defined and run by the Go side."""

    name: str
    description: str
    parameters: dict  # JSON schema of the arguments


class ToolCall(BaseModel):
    name: str
    arguments: dict = {}


class ToolResult(BaseModel):
    name: str
    content: str


class Step(BaseModel):
    """One assistant message with tool calls, and the results Go sent back."""

    calls: list[ToolCall]
    results: list[ToolResult]


class Turn(BaseModel):
    """A failed attempt (its tool steps, then its answer) and the
    validator's feedback on it."""

    steps: list[Step] = []
    answer: Answer
    feedback: str


class TranslateRequest(BaseModel):
    english: str
    model: str = DEFAULT_MODEL
    context: Context
    history: list[Turn] = []
    tools: list[Tool] = []
    steps: list[Step] = []  # tool work of the current attempt so far
    final: bool = False  # ask for the answer now, not for more tool calls
    num_ctx: int = 8192
    temperature: float = 0.0
    seed: int | None = None  # with temperature > 0: the same seed, the same sample


class TranslateResponse(BaseModel):
    """Either tool calls to run, or (no tools, or final) the answer.
    With tools and no calls, the model's plain reply is in content and the
    caller asks again with final."""

    answer: Answer | None = None
    calls: list[ToolCall] = []
    content: str = ""
    model: str
    prompt_tokens: int
    eval_tokens: int
    duration_ms: int


# --- Prompt building (pure functions, tested without a model).


def system_prompt(ctx: Context, tools: list[Tool] = []) -> str:
    """The system prompt. Stable parts first (rules, grammar, examples),
    then the dictionary, so consecutive requests share a long prefix and
    Ollama can reuse its KV cache for it."""
    return templates.get_template("system.j2").render(ctx=ctx, tools=tools)


def user_prompt(english: str) -> str:
    return templates.get_template("user.j2").render(english=english)


def step_messages(steps: list[Step]) -> list[dict]:
    """Per step: the assistant's tool calls, then one tool message per result."""
    msgs = []
    for st in steps:
        msgs.append(
            {
                "role": "assistant",
                "content": "",
                "tool_calls": [{"function": {"name": c.name, "arguments": c.arguments}} for c in st.calls],
            }
        )
        for r in st.results:
            msgs.append({"role": "tool", "content": r.content, "tool_name": r.name})
    return msgs


def build_messages(req: TranslateRequest) -> list[dict]:
    """system, user, then per failed attempt its tool steps, its answer and
    the feedback; then the current attempt's steps, and with final a request
    for the answer."""
    msgs = [
        {"role": "system", "content": system_prompt(req.context, req.tools)},
        {"role": "user", "content": user_prompt(req.english)},
    ]
    for turn in req.history:
        msgs += step_messages(turn.steps)
        msgs.append({"role": "assistant", "content": turn.answer.model_dump_json()})
        msgs.append(
            {
                "role": "user",
                "content": templates.get_template("feedback.j2").render(
                    feedback=turn.feedback, english=req.english
                ),
            }
        )
    msgs += step_messages(req.steps)
    if req.final and req.tools:
        msgs.append({"role": "user", "content": templates.get_template("final.j2").render(english=req.english)})
    return msgs


def ollama_tools(tools: list[Tool]) -> list[dict]:
    return [
        {"type": "function", "function": {"name": t.name, "description": t.description, "parameters": t.parameters}}
        for t in tools
    ]


# --- Coining new roots (POST /coin).


class Candidate(BaseModel):
    proto: str = Field(description="The proto-language root, letters only, without *.")
    reason: str = Field(description="One short sentence: why this form suits the meaning.")


class Coinage(BaseModel):
    """The answer schema of /coin."""

    definition: str = Field(description="A one-sentence dictionary definition of the concept.")
    candidates: list[Candidate] = Field(description="The proposed roots, all different.")


class Concept(BaseModel):
    english: str  # the gloss: "owl"
    pos: str
    gender: str = ""
    definition: str = ""  # given by the user; empty: the model writes one


class Root(BaseModel):
    """An existing entry, shown to the model as an example of the style."""

    proto: str
    modern: str
    pos: str
    gloss: str


class Round(BaseModel):
    """An earlier round: the model's candidates and the validator's verdicts."""

    coinage: Coinage
    feedback: str


class CoinRequest(BaseModel):
    concept: Concept
    phonology: str  # the proto sounds and phonotactics, written by Go
    examples: list[Root] = []
    history: list[Round] = []
    n: int = 5
    model: str = DEFAULT_MODEL
    num_ctx: int = 8192
    temperature: float = 0.0
    seed: int | None = None


class CoinResponse(BaseModel):
    coinage: Coinage
    model: str
    prompt_tokens: int
    eval_tokens: int
    duration_ms: int


def coin_messages(req: CoinRequest) -> list[dict]:
    """system (rules, phonology, examples), user (the concept), then per
    earlier round the model's answer and the feedback on it."""
    msgs = [
        {
            "role": "system",
            "content": templates.get_template("coin_system.j2").render(
                phonology=req.phonology, examples=req.examples
            ),
        },
        {"role": "user", "content": templates.get_template("coin_user.j2").render(c=req.concept, n=req.n)},
    ]
    for r in req.history:
        msgs.append({"role": "assistant", "content": r.coinage.model_dump_json()})
        msgs.append(
            {
                "role": "user",
                "content": templates.get_template("coin_feedback.j2").render(
                    feedback=r.feedback, c=req.concept, n=req.n
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


def chat(model: str, messages: list[dict], options: dict, **kwargs):
    """One Ollama call, with its errors mapped to HTTP status codes."""
    try:
        return client().chat(model=model, messages=messages, options=options, **kwargs)
    except ollama.ResponseError as e:
        raise HTTPException(502, f"ollama: {e.error}")
    except ConnectionError as e:  # the ollama client's wrapper for httpx.ConnectError
        raise HTTPException(503, f"ollama not reachable at {OLLAMA_HOST}: {e}. Start it with: ollama serve")
    except Exception as e:
        raise HTTPException(502, f"ollama call failed: {type(e).__name__}: {e}")


def sampling(temperature: float, num_ctx: int, cap: int, seed: int | None) -> dict:
    options = {"temperature": temperature, "num_ctx": num_ctx, "num_predict": cap}
    if seed is not None:
        options["seed"] = seed
    return options


def usage(model: str, resp, start: float) -> dict:
    return dict(
        model=model,
        prompt_tokens=resp.prompt_eval_count or 0,
        eval_tokens=resp.eval_count or 0,
        duration_ms=int((time.monotonic() - start) * 1000),
    )


@app.post("/translate")
def translate(req: TranslateRequest) -> TranslateResponse:
    start = time.monotonic()
    # Tools and format don't mix: constrained decoding would mask the tokens
    # of a tool call. So a tool round has tools and no format, and the
    # answer comes in a separate final call with format and no tools.
    use_tools = bool(req.tools) and not req.final
    kwargs = {"tools": ollama_tools(req.tools)} if use_tools else {"format": Answer.model_json_schema()}
    cap = TOOL_ROUND_MAX_TOKENS if use_tools else ANSWER_MAX_TOKENS
    # Rendered before the call: a template error is a bug here, not Ollama
    # being down, and FastAPI reports it as a 500 with the traceback logged.
    messages = build_messages(req)
    resp = chat(req.model, messages, sampling(req.temperature, req.num_ctx, cap, req.seed), **kwargs)

    done = usage(req.model, resp, start)
    raw = resp.message.content or ""
    if getattr(resp, "done_reason", "") == "length":
        log.warning("reply cut at %d tokens (%s round): %r", cap, "tool" if use_tools else "answer", raw[-200:])
    if use_tools:
        calls = [
            ToolCall(name=c.function.name, arguments=dict(c.function.arguments or {}))
            for c in resp.message.tool_calls or []
        ]
        return TranslateResponse(calls=calls, content=raw, **done)
    try:
        answer = Answer.model_validate(json.loads(raw))
    except (json.JSONDecodeError, ValidationError) as e:
        raise HTTPException(502, f"model reply is not a valid answer: {e}\nraw: {raw[:500]}")

    return TranslateResponse(answer=answer, **done)


@app.post("/coin")
def coin(req: CoinRequest) -> CoinResponse:
    start = time.monotonic()
    messages = coin_messages(req)
    resp = chat(
        req.model,
        messages,
        sampling(req.temperature, req.num_ctx, ANSWER_MAX_TOKENS, req.seed),
        format=Coinage.model_json_schema(),
    )
    raw = resp.message.content or ""
    try:
        coinage = Coinage.model_validate(json.loads(raw))
    except (json.JSONDecodeError, ValidationError) as e:
        raise HTTPException(502, f"model reply is not a valid coinage: {e}\nraw: {raw[:500]}")
    return CoinResponse(coinage=coinage, **usage(req.model, resp, start))
