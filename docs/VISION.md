# Vision

Lingua Nova is two things:

1. **A naturalistic constructed language**: one that feels like a real human
   language, with irregular forms and a history behind them.
2. **A Go toolkit and web app** for building, browsing, translating, and
   (eventually) talking in that language with the help of an LLM.

The project is also a learning vehicle. Its learning goals, in priority order:

- **Linguistics**: phonology, morphology, syntax, historical sound change.
- **Parsing & algorithms**: rule languages, tokenizers, morphological
  analysis, rule-based translation.
- **AI / LLMs**: prompting with structured context, tool use, validation
  loops, embeddings.

Web/backend work is deliberately kept simple (Go stdlib + htmx), because it
isn't a learning goal.

## Core idea: the language has a history

Naturalistic languages are irregular because they *evolved*. We copy that
process instead of inventing irregularity by hand:

```
proto-language roots  ──(sound change rules)──▶  modern words
   *akwa                 k > g / V_V                awa
   *kenta                a > e / _#                 tʃente
```

- Words are written in a **proto-language**: regular, clean, easy to design.
- A **sound change engine** applies ordered historical rules to get the modern
  forms. Irregularities fall out naturally, the way they did in real languages.
- The dictionary can show *why* a word looks the way it does (its derivation).

This also connects the three learning goals: the rules are linguistics, the
engine is parsing and algorithms, and the derivations become context for the AI.

## Components

| Component | What it does | Main learning |
|---|---|---|
| `language/` | The language spec: phonology, grammar, lexicon. Plain text in git; this is the source of truth. | Linguistics |
| Word generator | Invents new roots that obey phonotactics. | Phonology, randomness with constraints |
| Sound change engine | Parses rules like `k > g / V_V` and applies them in order. | Rule DSL parsing, pattern matching |
| Morphology | Inflects words (generate) and breaks them apart (analyze). | Finite-state morphology |
| Dictionary web app | Search both directions, show derivations and inflection tables. | Light: Go server + htmx |
| Rule-based translator | Translates a controlled subset of English, and glosses conlang text. | Tokenizing, parsing, transfer |
| AI layer | LLM translation checked by our own analyzer, semantic search, word suggestions, chat partner. | Prompting, tool use, evals, embeddings |

## Key design decisions

- **Language data lives in text files (YAML/Markdown) in git**, not a database.
  Every change to the language is a reviewable diff with history. The app
  loads the files at startup; a database is added only if one is ever needed.
- **Rule-based first, AI second.** The rule-based tools define what "correct"
  means. The AI layer then uses them as tools and validators, so its output
  can be checked and isn't trusted blindly.
- **Start tiny.** A working language with 100 roots and 5 grammar rules beats a
  half-designed one with 2000 words.
