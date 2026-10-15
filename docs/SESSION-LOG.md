# Session log

Newest first. One entry per session: what was done, what was decided and
why, and where to pick up. The task list itself lives in `ROADMAP.md` →
"Next up"; this log gives the context behind it.

---

## 2026-10-15 (evening): Phase 2, step 2.3, lexicon validator

### Done

- `internal/phon/phonotactics.go`: `CheckPhonotactics` (per-syllable onset
  and coda checks, word-final codas from `finals`) and `CheckForbidden` (a
  flat scan, because forbidden sequences cross syllable boundaries; category
  items like `[V, V, V]` work).
- `internal/lexicon`: `Load` decodes through `yaml.Node` to keep line
  numbers; unknown keys are reported. `Validate` collects every problem,
  sorted by file and line, in the format `file:line: id: message`.
- `lnova validate` → `104 entries, 0 problems`, exit 1 on any problem.
- Tests: phonotactics table, one validator case per rule, parse errors, the
  real lexicon validates clean, and the CLI on a broken temp lexicon.

### Key decisions

| Area | Decision | Why |
|---|---|---|
| Gender | Required on nouns, **optional on pronouns**, an error elsewhere | User's choice: `sela` keeps `gender: f` from the noun "honour" it came from |
| Proto checks | Proto-only sounds + `forbidden` only, no onset/coda check | Proto has geminates (`*pippo`, `*ruccar`, `*cuppe`); real proto phonotactics is still to be designed (before 2.6) |
| `pos` | Closed list of the 11 values in use | Typos (`nown`) are caught; add a value to `lexicon.PartsOfSpeech` when needed |
| Verb endings | Checked on both proto and modern | Both are infinitives |

### Start next session here

1. `ROADMAP.md` → "▶ Next up": step 2.4, sound change rule parser.
2. Steps 2.1–2.3 are **not committed yet**, unless the user committed at the
   end of this session.

---

## 2026-10-15 (later): Phase 2, step 2.2, syllables and stress

### Done

- `internal/phon/syllable.go`: `Syllabify` (onset/nucleus/coda),
  `Stress` (penultimate), `SyllableIPA` (`naˈɾa.ʃu`), `SyllableSpelling`
  (`na.ra.xu`).
- `lnova ipa folha` now prints `folha   f·o·lh·a   /ˈfo.ʎa/`. A word with no
  vowel is an error.
- `forbidden` in `phonology.yaml` may now contain category names. Category
  names must start with an uppercase letter so they can't clash with a
  segment spelling.
- Tests: syllabification table, no-vowel error, and every lexicon form
  syllabifies and joins back to the original word.

### Key decisions (with the user's reasons)

| Area | Decision | Why |
|---|---|---|
| V-V-V | **Banned** (new phonotactic rule, `forbidden: [V, V, V]`) | Simplest option: no rule needed for how `aia` splits, and the generator drops such words. No current word has one |
| Monosyllables | No stress mark in IPA (`/lu/`) | Usual dictionary practice; the mark tells you nothing |
| Syllabifier scope | Structural parser only; illegal codas (`*cuppe` → `cup.pe`) are split anyway | Checking legality is the validator's job (2.3), the same split as parse vs. type-check |

### Start next session here

1. `ROADMAP.md` → "▶ Next up": step 2.3, lexicon loader + `lnova validate`.
2. Steps 2.1 and 2.2 are **not committed yet**.

---

## 2026-10-15: Phase 1 finished

### Done

- Three hand translations with interlinear glosses in `grammar.md`
  → Example sentences, plus a glossing convention: verbs are
  **stem-TH-TENSE-PERSON**, with past `-x-` and future `-r-` (the old
  infinitive `-r`, reanalysed as a tense marker in the same slot).
- Settled all Phase 1 open points (table below).
- Added lexicon entry `rifatian` (*rifatia → rifacha), 101 entries now.
- First real commit of the language core.

### Key decisions (with the user's reasons)

| Area | Decision | Why |
|---|---|---|
| Word-final `h` | Allowed in any word, not only in endings | Naturalistic (like English final -s in *bus*); the Phase 4 analyzer must handle the ambiguity, which is a useful parsing lesson |
| Polite plural | **selah** / selah / selahes, with a 3pl verb; `chuh` is informal only | Follows Brazilian *você → vocês*; `sela` already acts as a noun, so the regular `-h` plural is expected |
| Extra tenses | Postponed until after Phase 4 | Not needed for Phase 2; the morphology generator will show what's cheap to add |
| Native name | **rifacha** < *rifatia (confirmed) | Gives the engine one more rule-1 test case |
| Question words | who `ifa`, what `itu`, where `iba` exist; when/why/how **not coined yet** | Coining words is the user's call |

- Later in the session the user asked Claude to coin when/why/how starting
  with `cu-`: **cuda** < *cutan, **cuxa** < *cusia, **cupe** < *cuppe
  (104 entries). This creates a second question-word series: `i-` for
  participants and places, `cu-` for circumstances.

### Phase 2, step 2.1 (same session)

- `language/phonology.yaml`: segments (`modern_only` marks the palatals),
  diphthongs, onset clusters, codas, finals, forbidden sequences. V and C
  categories are built automatically; finer ones go under `categories:`.
  Sequences are YAML lists (`[l, h]`), not strings, because `"lh"` would
  tokenize as the single segment /ʎ/.
- `internal/phon`: `Load`/`Parse` with validation (unknown or mis-kinded
  segments in sequences, duplicates, unknown YAML fields). `Tokenize` uses
  greedy longest match and returns `*UnknownLetterError` with a byte position.
  Empty codas/finals are allowed (a pure-CV language is valid).
- `lnova [-lang dir] ipa <word>...` prints `folha   f·o·lh·a   /foʎa/`. A
  leading `*` is accepted.
- Dependency: `go.yaml.in/yaml/v3` (maintained fork of `gopkg.in/yaml.v3`).
- Tests: tokenizer table, error positions, parse errors, the CLI, and every
  lexicon `proto`/`modern` form tokenizes.

### Start next session here

1. `ROADMAP.md` → "▶ Next up": Phase 2, step 2.2, syllabifier + stress.

---

## 2026-10-14: Project setup and language design (Phase 0–1)

### Done

- Set up the repo structure: `docs/VISION.md`, `docs/ROADMAP.md`, `language/`,
  `go.mod` (`github.com/olehmushka/lingua-nova`, Go 1.26), `CLAUDE.md`.
- Designed the phonology, spelling, sound changes and grammar of **Rifatian**
  (`language/*.md`, `language/sound-changes.txt`).
- The user wrote the **100 lexicon entries** (`language/lexicon/*.yaml`, 9 files)
  and extended the entry format with `modern` (expected derivation result)
  and `gender`.
- Wrote a detailed Phase 2 plan (steps 2.1–2.6) in `ROADMAP.md`.

### Key decisions (with the user's reasons)

| Area | Decision | Why |
|---|---|---|
| Project shape | Language data as text in git → Go CLI → web dictionary → rule-based translator → AI layer | Rule-based tools define "correct"; the AI uses them as tools and validators |
| Stack | Go stdlib + htmx | User is a senior Go dev; web isn't a learning goal |
| Learning goals | Linguistics, parsing/algorithms, AI/LLMs | Explain these concepts; don't explain Go |
| Language style | **Naturalistic**, **fully invented** vocabulary (not from Latin) | User's choice |
| Irregularity | Invented **proto-language** + ordered **sound changes** → modern forms | Lets a naturalistic language still be rule-based |
| Sound | Brazilian Portuguese rhythm (clear vowels, open syllables), Occitan/Portuguese feel, **no nasal endings**, Sumerian **/x/** spelled `h`; everything easy for a Ukrainian speaker | User's brief |
| Spelling | English letters only, Portuguese/Occitan digraphs: `lh nh ch x j`; `c` and `g` always hard; `s` always /s/; no k q w y | Romance look, one reading per spelling |
| Stress | Always penultimate, no exceptions | No accent marks, so stress must be predictable |
| Grammar | SVO; invariable English-like articles (`lu`, `ne`); 2 genders without agreement; plural `-h/-eh`; adjectives after the noun; possession with `-s` like English *'s*; pronouns with subject/object/possessive forms incl. polite *você*-like `sela` (3sg verb) and gendered "they" | User's answers |
| Verbs | 3 classes (`-ar/-er/-ir`), stem + theme vowel + tense + person; past `-x-`, future = infinitive + endings; pro-drop | Romance conjugation, agglutinative build |
| Negation / questions | Portuguese-style: `na` before the verb, double negation, yes/no by intonation, answer "yes" by repeating the verb | User's choice |
| Name | **Rifatian** (English name). Suggested native name *rifacha* < *\*rifatia*, **not yet confirmed** | |

### Changes Claude made late in the session (tell the user if they object)

- Added the phonotactic rule "**`l`, `n`, `c` are never followed by `h`**" to
  `phonology.md`. Because `h` is /x/, `lh` would otherwise be ambiguous.
  No current lexicon word violates it.
- Fixed a stress example in `phonology.md`: `narexu` → `naraxu` (class I past).

### State at end of session

- **Nothing is committed.** Everything except the initial README is untracked or modified.
- Phase 1 is almost done: hand translations and open points remain.
- No Go code yet.

### Open questions (carried over)

- Is word-final `h` allowed in all words, or only in endings?
- Polite plural: `chuh` or `selah`?
- Question words: they may already be in `pronouns.yaml`; check, then close the item in `grammar.md`.
- Extra tenses (perfect / conditional / subjunctive): decide or postpone.
- Native name of the language (*rifacha*?).

### Start next session here

1. `ROADMAP.md` → "▶ Next up": finish Phase 1 (3 glossed translations,
   open questions above, commit).
2. Then Phase 2, step 2.1: `language/phonology.yaml` + tokenizer.
