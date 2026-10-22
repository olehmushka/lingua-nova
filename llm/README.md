# llm/: local LLM service

A small FastAPI service between `lnova llm` (Go) and a local model in
[Ollama](https://ollama.com). Nothing leaves the machine.

One request = one model call: it renders the prompt (`prompts/*.j2`),
calls Ollama, and returns either tool calls or an answer. An answer is
generated with its JSON schema as `format` (constrained decoding) and
validated with pydantic. The retry loop, the tool loop, the tools and the
validator all live in Go (`internal/llm`), because the rule-based tools
decide what counts as correct.

## Run

```sh
ollama serve                                   # terminal 1 (:11434)
ollama pull qwen2.5:7b-instruct-q4_K_M         # once
cd llm && uv run uvicorn app:app --port 8765   # terminal 2
curl localhost:8765/health
lnova llm -compare "The men will drink the water."
lnova llm -tools -ctx none "The dog eats the bread."   # the model looks words up
lnova eval -list                               # the eval set, no model
lnova eval -runs 5 -out evals/results/x.json   # pass rates per setup (hours)
lnova eval -show evals/results/x.json          # report a saved run; add -resume to continue one
lnova llm -temp 0.7 -seed 3 "…"                # replay one eval run (run 3)
lnova coin -pos noun -gender f lantern         # coin proto roots; YAML on stdout
```

## Endpoints

- `GET /health?model=…`: checks that Ollama is up and the model is pulled.
- `POST /translate`: `{english, model, context:{grammar, morphology,
  entries, examples}, history:[{steps, answer, feedback}], tools, steps,
  final, num_ctx, temperature, seed}` returns `{answer:{translation,
  words:[{text, lexicon_id, features}], notes}, calls, content, model,
  prompt_tokens, eval_tokens, duration_ms}`. A reply that doesn't fit the
  schema returns 502, and Ollama being down returns 503. `temperature` 0
  (the default) is greedy decoding; above 0 the model samples, and `seed`
  (optional) makes the sample repeatable.

- `POST /coin`: `{concept:{english, pos, gender, definition}, phonology,
  examples:[{proto, modern, pos, gloss}], history:[{coinage, feedback}], n,
  model, num_ctx, temperature, seed}` returns `{coinage:{definition,
  candidates:[{proto, reason}]}, model, prompt_tokens, eval_tokens,
  duration_ms}`. The phonology text and the examples are built in Go
  (`internal/llm/coin.go`), which also checks every candidate; prompts in
  `prompts/coin_*.j2`. Same errors as `/translate`.

### Tools

`tools` is `[{name, description, parameters}]` (a JSON schema, defined in
Go). A step is `{calls:[{name, arguments}], results:[{name, content}]}`:
one assistant message with tool calls, then one `tool` message per result.

- `tools` set and `final` false: Ollama gets `tools` and no `format`.
  The reply is `calls` to run, or no calls and the model's text in
  `content` (it's done). `answer` is null.
- No `tools`, or `final` true: Ollama gets `format` and no `tools`, and the
  reply is an `answer`. With tools, a last user message asks for it.

The two can't be combined: constrained decoding to the schema would mask
the tokens of a tool call. Go runs the calls, appends the step to `steps`
and calls again, then sends `final`.

## Test

```sh
uv run pytest      # prompt rendering, message order, schema; no model needed
```
