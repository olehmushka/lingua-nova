# Roadmap

Each phase ends with something that works. Don't start a phase until the
previous one is "done", even if done means small.

## ▶ Next up

_Keep this section current: it's where every new session starts._

1. **Phase 2, step 2.5**: rule engine + `lnova evolve`. `internal/sound`
   already has `Rule.Apply` (one rule, simultaneous). Next comes ordered
   application of all rules, the derivation trace, the CLI, and the whole
   lexicon as a table test. Every mismatch with `modern` gets reported to the
   user, never "fixed" silently.
2. Before 2.6 (generator): decide the **proto phonotactics**. Today proto
   forms are only checked for proto-only sounds and `forbidden`, because
   proto has geminates (`*pippo`) that modern codas don't allow.

## Phase 0 — Learn the basics (reading, ~1–2 weeks alongside Phase 1)

- *The Language Construction Kit*, Mark Rosenfelder (zompist.com). The
  classic practical guide.
- *The Art of Language Invention*, David J. Peterson (who made Dothraki).
- YouTube: **Biblaridion**, "Make a Language" series (naturalistic, with
  sound change), and **Artifexian**.
- Learn to read the IPA chart. You'll use it everywhere.

## Phase 1 — Design the language core (no code)

Files in `language/`.

- [x] Phonology: consonant and vowel inventory (IPA), syllable structure, stress.
- [x] Romanization: how each sound is spelled in Latin letters.
- [x] ~100 proto-language roots (basic vocabulary: body, nature, pronouns, numbers 1–5, common verbs).
- [x] 10–20 sound change rules, written in plain text (14 starter rules).
- [x] Basic grammar: word order, nouns (number, case?), verbs (tense/aspect, agreement?).
- [x] Translate 3 simple sentences by hand, with interlinear glosses, into `grammar.md` → Example sentences:
  - "The woman sees the river." (present, articles)
  - "The men will drink the water." (plural, future, verb class)
  - "Didst thou see the woman's dog?" (question, past, possession)
- [x] Settle the open points:
  - `phonology.md`: word-final `h` is allowed in all words.
  - `grammar.md`: polite plural is `selah` (3pl verb); `chuh` is informal only. Question words: `i-` series (who/what/where) + `cu-` series (when `cuda`, how `cuxa`, why `cupe`).
  - Extra tenses (perfect, conditional, subjunctive): postponed until after Phase 4.
- [x] Commit.

**Done when:** you can translate "The woman sees the river." by hand and explain every piece.

## Phase 2 — Go toolkit: `lnova` CLI

Build in this order; each step is usable on its own. Every step ends with
`go test ./...` passing and a CLI command you can run.

### 2.1 Machine-readable phonology + tokenizer

- [x] `language/phonology.yaml`: the inventory that `phonology.md` describes
  in prose, as data. It holds segments (spelling, IPA, consonant/vowel,
  proto-or-modern-only), categories (`V`, `C`, and later finer ones like
  stops), diphthongs, onset clusters, allowed codas and word-finals, and
  forbidden sequences (`l/n/c + h`).
  *Why a file and not Go constants:* the validator, generator and engine all
  share it, and a second daughter language later only needs a new file.
- [x] `internal/phon`: load it, plus a **tokenizer** that splits spelling into
  segments with greedy longest match (`folha` → `f o lh a`, not `f o l h a`).
- [x] `lnova ipa <word>` → `folha   f·o·lh·a   /ˈfo.ʎa/` (dots and stress since 2.2).

*Concept to learn:* **segments vs. letters.** All the linguistic logic works on
sounds, and spelling is just one way of writing them.

### 2.2 Syllabification and stress

- [x] Syllabifier using the **maximal onset principle**: a consonant between
  vowels joins the *following* syllable when the result is a legal onset
  (`pa.tra`, not `pat.ra`).
- [x] Diphthongs as one nucleus; other vowel pairs split (`di.a`).
- [x] Stress on the penultimate syllable.

### 2.3 Lexicon loader + `lnova validate`

- [x] `internal/lexicon`: load all `lexicon/*.yaml`.
- [x] Checks:
  - duplicate ids; required fields; `gender` on nouns.
  - verbs end in `-ar/-er/-ir`.
  - `proto` uses only proto segments (no palatals).
  - `modern` obeys the phonotactics (onsets, codas, word-final, forbidden sequences).
- [x] Error messages include file and line, like a compiler.
- [x] `lnova validate` prints `104 entries, 0 problems`; exits 1 on any problem.

### 2.4 Sound change rule parser

- [x] Parse `sound-changes.txt` into an AST: target, replacement, environment
  (`_`, `#`, categories, `∅`), with line numbers for errors.
- [x] Decide and document the **application semantics** before coding.
  Does a rule match against the original form (simultaneous application) or
  against the form as it's being rewritten (iterative)? Real sound-change
  tools differ here. Write a test that shows the difference.
  → **Simultaneous**, documented in the header of `sound-changes.txt`;
  `TestApplySimultaneous` (`a > e / e_`: `eaaa` → `eeaa`, not `eeee`).
- [x] `lnova rules` lists the parsed rules with their line numbers.

*Concept to learn:* rule ordering. Rules can **feed** each other (one creates
the input for the next) or **bleed** each other (one removes it). Rules 2 and 3
in `sound-changes.txt` are a bleeding example.

### 2.5 Rule engine + `lnova evolve`

- [ ] Apply rules in order to segment slices and record a **derivation trace**
  (which rule changed what).
- [ ] `lnova evolve`: run every lexicon `proto` and compare with `modern`;
  print mismatches; exit non-zero on any mismatch (CI-friendly).
- [ ] `lnova evolve --trace <id>`: `*esia → exa  (line 28: si > x / _V)`.
  Rules are identified by line number (decided in 2.4).
- [ ] A test that runs the whole lexicon as a table-driven test.

### 2.6 Word generator

- [ ] `lnova gen -n 20`: random proto roots built from weighted syllable
  templates, filtered through the 2.3 validator, skipping existing words.
  Show each candidate with its evolved modern form.

**Done when:** `lnova evolve` reproduces all 100 `modern` forms (or every
mismatch has been explained and fixed in the data or the rules) and can print
the derivation.

## Phase 3 — Dictionary web app

- [ ] Go `net/http` + `html/template` + htmx.
- [ ] Search conlang→English and English→conlang.
- [ ] Entry page: meaning, part of speech, examples, derivation history.
- [ ] Hot-reload `language/` files while you edit them.

**Done when:** you prefer using the app over grepping YAML.

## Phase 4 — Morphology and rule-based translation

- [ ] **Generator**: root + features → inflected form (`see + PAST + 3SG`).
- [ ] **Analyzer**: inflected form → root + features (the reverse; harder).
- [ ] **Interlinear glossing**: conlang sentence → word-by-word gloss line.
- [ ] **English → conlang** for a controlled subset (simple SVO sentences): tokenize, tag, build a small tree, reorder per grammar, inflect.

**Done when:** the app glosses any sentence you write and translates simple English sentences correctly.

## Phase 5 — AI layer (Claude API via the Go SDK)

- [ ] **LLM translation**: grammar + relevant dictionary entries in the prompt, then the Phase 4 analyzer checks the output. Retry with the errors if it fails.
- [ ] **Tool use**: the LLM calls your Go functions (`lookup`, `inflect`, `analyze`) instead of guessing.
- [ ] **Coin new words**: the LLM proposes roots for missing concepts, and the validator + sound change engine accept or reject them.
- [ ] **Semantic search** with embeddings ("words related to the sea").
- [ ] **Evals**: a fixed set of test sentences to measure translation quality as you change prompts.
- [ ] Stretch: a chat partner that speaks only the conlang.

**Done when:** LLM translations pass your validator most of the time, and you can measure that number.

## Later ideas

- Custom script (font or SVG glyphs).
- A second daughter language from the same proto-language.
- Text-to-speech from IPA.
- Corpus: write stories and use them as examples for the AI.
