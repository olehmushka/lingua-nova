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
