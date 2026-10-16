# lingua-nova

**Rifatian**, a naturalistic constructed language, together with Go tools to build, browse,
and translate it, and an LLM layer on top.

- [Vision](docs/VISION.md): what this is and the key design decisions
- [Roadmap](docs/ROADMAP.md): phases and what "done" means for each
- [The language](language/README.md): the language spec itself

## Layout

```
language/   the language spec (source of truth, hand-edited)
docs/       vision and roadmap
cmd/        (Phase 2+) lnova CLI, web server
internal/   (Phase 2+) phonology, sound changes, lexicon, morphology, translation, ai
```

## Run

```
go run ./cmd/lnova validate      # check the lexicon (see `lnova` for all commands)
go run ./cmd/lnova evolve        # proto → modern for every entry
go run ./cmd/server              # dictionary on http://localhost:8080
```
