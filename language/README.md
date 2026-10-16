# Rifatian

This directory is the source of truth for the language. The Go tools read
these files, and you edit them by hand.

**Rifatian** is the English name. Native name: **rifacha**
(< proto *\*rifatia*, rule 1: ti > ch / _V; lexicon id `rifatian`).

| File | Contents |
|---|---|
| `phonology.md` | Sounds, syllable structure, stress, romanization |
| `phonology.yaml` | The same inventory as data, for the Go tools (keep in sync with `phonology.md`) |
| `sound-changes.txt` | Ordered historical rules: proto → modern |
| `grammar.md` | Word order, morphology, syntax |
| `morphology.yaml` | The inflection tables of `grammar.md` as data, for `lnova inflect` and `lnova analyze` (keep in sync with `grammar.md`) |
| `lexicon/*.yaml` | Dictionary entries, grouped by topic |

## Lexicon entry format (draft, expect it to change)

```yaml
- id: water            # stable English-ish key, unique
  proto: acua          # proto-language root, romanized (same spelling as modern)
  modern: agua         # hand-derived modern form; the engine must reproduce it
  # word: awa          # optional: override the derived modern form
  pos: noun
  gender: f            # nouns only: m or f
  gloss: water         # short meaning, used in glossing
  definition: Water; any liquid in general.
  examples:
    - text: ...
      translation: ...
  notes: "*acua → agua (rule 2: c > g / V_V)"
```

The modern word is normally *derived* by running `proto` through
`sound-changes.txt`. Set `word` only when you want a manual exception.

`modern` is not an override: it records what the derivation *should* give.
Phase 2's `lnova evolve` checks every entry against it, so the lexicon
doubles as the sound-change engine's test table. If you change a rule,
expect some `modern` values to need updating too.

Run `go run ./cmd/lnova validate` after editing. It checks required fields
(`id proto modern pos gloss definition`), duplicate ids, `pos` (one of
adjective, adverb, article, conjunction, determiner, noun, numeral, particle,
preposition, pronoun, verb), `gender` (`m`/`f`; required on nouns, optional
on pronouns), verb endings, proto sounds, and modern phonotactics.

Verbs are listed by their proto infinitive (`narar`); the class follows from
the theme vowel. Pronouns list only the singular subject form; the rest of
the paradigm is in `grammar.md`.
