# lingua-nova

**Rifatian**: a naturalistic constructed language, plus Go tools to build,
browse and translate it, plus (later) an LLM layer. See `docs/VISION.md`.

## Where we are / what to do next

**Read `docs/ROADMAP.md` first.** Its "Next up" section is the current task
list. Work through it in order, tick checkboxes as items are done, and keep
"Next up" pointing at the real next step before ending a session.

**Then read the latest entry in `docs/SESSION-LOG.md`** for the decisions,
open questions and context behind the task list. At the end of a session,
add a new entry at the top.

## The user

- Senior Go developer (also senior Node.js). Don't explain Go or web basics.
- New to linguistics and AI/LLMs. This is a **learning project**: explain
  the linguistics, parsing and AI concepts behind each step, and let the user
  make the design decisions about the language.
- Learning goals: linguistics, parsing/algorithms, AI/LLMs. The web part
  stays deliberately simple.

## Source of truth: `language/`

Hand-edited files that define the language. The Go code reads them and never
writes them.

- `phonology.md`: sounds, spelling, phonotactics, stress
- `phonology.yaml`: the same inventory as data (read by `internal/phon`)
- `grammar.md`: morphology and syntax; **(draft)** marks unsettled forms
- `sound-changes.txt`: ordered rules, proto → modern
- `lexicon/*.yaml`: 323 entries; format in `language/README.md`

Rules:

- **Ask before changing language design** (sounds, grammar, words, rules).
  Point out problems and propose fixes, but the user decides.
- In each lexicon entry, `modern` is the **expected output** of running
  `proto` through the sound changes. It's the test oracle for the engine.
  When the engine and `modern` disagree, report it; don't silently "fix" either side.
- Spelling is English letters only. `lh nh ch` are single sounds. `h` = /x/,
  `x` = /ʃ/, `j` = /ʒ/, `c` = /k/ always. Proto forms are written with `*`.

## Code conventions

- Go (see `go.mod`), stdlib-first. External deps only when clearly worth it
  (a YAML parser is fine).
- Layout: `cmd/lnova` (CLI), `cmd/server` (web, Phase 3), `internal/...`,
  `llm/` (Python FastAPI service in front of a **local** Ollama model; no
  external LLM APIs). `lnova llm` needs `ollama serve` + the service (see
  `llm/README.md`); the Go tests use a fake service and don't.
- Table-driven tests. The lexicon itself is a test fixture.
- `go test ./...` (and `cd llm && uv run pytest`) must pass before calling a step done.
