# llm/: local LLM service

A small FastAPI service between `lnova llm` (Go) and a local model in
[Ollama](https://ollama.com). Nothing leaves the machine.

One request = one translation attempt: it renders the prompt
(`prompts/*.j2`), calls Ollama with the answer's JSON schema as `format`
(constrained decoding), validates the reply with pydantic and returns it.
The retry loop and the validator live in Go (`internal/llm`), because
the rule-based tools decide what counts as correct.

## Run

```sh
ollama serve                                   # terminal 1 (:11434)
ollama pull qwen2.5:7b-instruct-q4_K_M         # once
cd llm && uv run uvicorn app:app --port 8765   # terminal 2
curl localhost:8765/health
lnova llm -compare "The men will drink the water."
```

## Endpoints

- `GET /health?model=…`: checks that Ollama is up and the model is pulled.
- `POST /translate`: `{english, model, context:{grammar, morphology,
  entries, examples}, history:[{answer, feedback}], num_ctx, temperature}`
  returns `{answer:{translation, words:[{text, lexicon_id, features}], notes},
  model, prompt_tokens, eval_tokens, duration_ms}`. A reply that doesn't
  fit the schema returns 502, and Ollama being down returns 503.

## Test

```sh
uv run pytest      # prompt rendering, message order, schema; no model needed
```
