# Roadmap

Each phase ends with something that works. Don't start a phase until the
previous one is "done", even if done means small.

## ▶ Next up

_Keep this section current: it's where every new session starts._

1. **Evals** (Phase 5): the 7-sentence measurement, made repeatable: more
   sentences, several runs each, a pass rate per setup.
2. **Coin new words** (Phase 5): the LLM proposes roots for missing
   concepts; the validator + sound change engine accept or reject them.
3. Open question from the lexicon growth (SESSION-LOG 2026-10-18, later):
   re-seed the generator weights from roots only, not derived words?

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

- [x] Apply rules in order to segment slices and record a **derivation trace**
  (which rule changed what).
- [x] `lnova evolve`: run every lexicon `proto` and compare with `modern`;
  print mismatches; exit non-zero on any mismatch (CI-friendly).
- [x] `lnova evolve --trace <id>`: `*esia → exa  (line 28: si > x / _V)`.
  Rules are identified by line number (decided in 2.4).
- [x] A test that runs the whole lexicon as a table-driven test.
  → `sound.Evolve` + `TestLexiconEvolves`; 104/104 entries match `modern`.

### 2.6 Word generator

- [x] Proto phonotactics decided and enforced (`phonology.yaml` → `proto:`;
  `lnova validate` checks proto onsets, codas, finals and geminates).

- [x] `lnova gen -n 20`: random proto roots built from weighted syllable
  templates, filtered through the 2.3 validator, skipping existing words.
  Show each candidate with its evolved modern form.
  → position-aware templates in `phonology.yaml` → `proto: generator:`,
  sounds weighted by lexicon frequency per slot, `internal/gen`;
  homophones marked (`*ciu → chu = thou`); `-seed` reproduces a batch.

- [x] Wrap-up: proto hiatus only after `i`; sound change rules 5–6 fix two
  palatalization side effects found by the generator; lexicon grown to 323
  roots (special families: kill, love, create, god).
- [x] Generator weights re-seeded from the 323 roots (`lnova gen -count`).

**Done when:** `lnova evolve` reproduces all 100 `modern` forms (or every
mismatch has been explained and fixed in the data or the rules) and can print
the derivation.

## Phase 3 — Dictionary web app

- [x] Go `net/http` + `html/template` (`cmd/server`).
- [x] htmx live search: the input sends `hx-get` as you type (200ms
  debounce) and gets back just the results fragment (`HX-Request` header);
  htmx 2.0.4 embedded in the binary, URL kept in sync.
- [x] Search conlang→English and English→conlang.
  → `lexicon.Search`: one box, both directions, `*` for proto; ranked
  exact > word > prefix > substring, then by field.
- [x] Entry page: meaning, part of speech, examples, derivation history.
  → IPA, derivation table, mismatch warning, homophones, source file:line.
- [x] Hot-reload `language/` files while you edit them.
  → polls size + mtime every 500ms (`-poll`); a load error keeps the old
  data under a red banner, validate problems show the new data under a
  yellow one.

**Done when:** you prefer using the app over grepping YAML. → Done (2026-10-17).

## Phase 4 — Morphology and rule-based translation

- [x] **Generator**: root + features → inflected form (`see + PAST + 3SG`).
  → `language/morphology.yaml` (verbs, nouns, pronouns), `internal/morph`;
  `lnova inflect see PST 1SG` / `lnova inflect see` (whole paradigm);
  paradigm table on the entry page; `validate` checks every form.
- [x] **Analyzer**: inflected form → root + features (the reverse; harder).
  → `morph.Analyzer`: affix stripping right to left, then verification by
  re-inflecting (analysis by synthesis); pronouns by table lookup; all
  analyses ranked, unknown roots as `?guesses`. `lnova analyze`; search
  shows "naraxu is a form of narar". Every paradigm cell round-trips.
- [x] **Interlinear glossing**: conlang sentence → word-by-word gloss line.
  → `internal/gloss`: a small Constraint Grammar on top of the analyzer
  (one verb per clause, SBJ before / OBJ after it, subject–verb
  agreement; never removes a word's last reading, notes it instead).
  `lnova gloss`, `/gloss` page with live htmx; grammar.md's examples are
  golden tests.
- [x] **English → conlang** for a controlled subset (simple SVO sentences): tokenize, tag, build a small tree, reorder per grammar, inflect.
  → prose tagger (averaged perceptron) + `language/english/words.yaml`
  for function words and archaic forms; `internal/earley` parses with
  `language/english/grammar.cfg` (all trees); `internal/translate`
  transfers (drop do-support, tense onto the verb, na, wh first,
  adjectives after the noun), inflects with agreement, lists **every**
  translation ranked, and verifies each with the glosser.
  `lnova parse`, `lnova translate`, `/translate` page; grammar.md's
  examples are golden tests.

**Done when:** the app glosses any sentence you write and translates simple English sentences correctly. → Done (2026-10-17).

## Phase 5 — AI layer (local model: Ollama + Python service in `llm/`)

- [x] **LLM translation**: grammar + relevant dictionary entries in the prompt, then the Phase 4 analyzer checks the output. Retry with the errors if it fails.
  → `llm/` FastAPI service (pydantic schema as Ollama `format`, Jinja
  prompts) in front of `qwen2.5:7b-instruct-q4_K_M`; `internal/llm`
  builds the context (`full`: all 323 entries; `retrieved`: closed classes +
  what the English words point to), checks each answer (glosser + the
  model's claimed ids/features) and retries with the gloss and problems
  (max 3). `lnova llm [-ctx] [-model] [-attempts] [-compare]`.
  First runs: almost everything passes, but only grammar.md's own examples
  are right. The validator checks form, not meaning.
  → **Meaning check**: when the English parses, the rule-based
  translations are references and the answer's clauses (wh, subject, na,
  verb, object; `gloss.Result.Clauses`) must equal one of them; otherwise
  a lexical check (every English content word's entry present, none
  added). Meaning problems fail the answer and go back as feedback.
  Glosser: a question word as subject agrees 3SG when the verb has an object.
- [x] **Tool use**: the LLM calls your Go functions (`lookup`, `inflect`, `analyze`) instead of guessing.
  → `internal/llm/tools.go`: `lookup`, `inflect` (takes an id or the
  dictionary word), `analyze`, `check` (the validator on a draft); plain-text
  results, errors returned to the model. Go runs the agent loop inside
  each attempt (tool rounds, max 8, then a `final` call with the JSON
  schema; tools and `format` can't share a call). `lnova llm -tools
  [-tool-rounds N] [-ctx none]`. First numbers (retrieved): 3/7 → 5/6 right
  with tools; the model sometimes gets the right form from `inflect` and
  still writes its own.
  → Measured on 629 entries (7 sentences, one run each): retrieved 3/7,
  retrieved + tools 6/7, none + tools 6/7 (the model looks words up
  itself, 1.5–2× the cost). `The dog eats the bread.` fails everywhere.
  Repeated identical calls are answered "repeated call: …" instead of
  run again (that doom loop: 120 → 22 calls).
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
