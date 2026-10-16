# Session log

Newest first. One entry per session: what was done, what was decided and
why, and where to pick up. The task list itself lives in `ROADMAP.md` →
"Next up"; this log gives the context behind it.

---

## 2026-10-16 (late night): generator re-seed, Phase 3 search + entry page

### Done

- **`lnova gen -count`** (`gen.Count`): syllabifies every proto root and
  prints the template counts as a `generator:` block. A geminate's first
  half counts as a coda (`*pip.po`: initial `CVC`).
- **Generator weights re-seeded from the 323 roots** (`phonology.yaml`,
  `phonology.md`). Biggest shift: initial `CVC` 13 → 101, now the most
  common first syllable (`lir.ma`, `bil.sa`, geminates). New shapes: mono
  `CVVC`, initial `CCVV`, medial `CCV`/`VC`. Reject rate is about 14% both
  before and after (3 × 300 words), still 0 illegal modern words.
- **`lexicon.Search`**: one query, both directions. Rifatian forms
  (`modern`, `word`, substring too), English `gloss` and `definition` (by
  whole word, so `ate` doesn't find `water`), `*` = proto only. Gloss dots
  count as spaces (`accident` → `kill.by.accident`).
- **`cmd/server`**: `net/http` + embedded `html/template`, no JS.
  `/?q=` search, `/entry/{id}` with IPA, proto IPA, the derivation table
  (rule line, rule, before → after), a red warning if the rules don't
  give `modern`, homophones, source file:line. Everything is computed once
  at start. `httptest` tests over the real `language/`.
- `phon.Spell` replaces the CLI's private `spell`.

### Key decisions

| Area | Decision | Why |
|---|---|---|
| Weights | **New counts as-is** (user's choice) | The generator should imitate the lexicon as it is now |
| Ranking | exact > whole word > prefix > substring, then field tier (modern/word, gloss, proto, definition), then alphabetical | A whole-word gloss hit (`love` in `love.eros`) must beat a prefix; a stable order makes tests and pages predictable |
| Search index | Linear scan | 323 entries take microseconds; an inverted index (word → entries) is the step if the lexicon grows a lot |
| Server data | Loaded once at start | Hot reload is the next step |

### Concepts

- **Ranking as a tuple sort**: each hit gets (match kind, field tier,
  headword) and results sort on that, the same idea search engines use
  with many more features and learned weights.
- **Normalization**: queries and fields go through the same function
  (lower-case, non-letters → spaces), so `Water;`, `water` and
  `spring.water` compare as words.

### Start next session here

1. `ROADMAP.md` → "▶ Next up": hot reload, then htmx live search.
2. Committed.

---

## 2026-10-16 (night): Phase 2 wrap-up, lexicon 104 → 323

### Done

- **Proto hiatus only after `i`** (`phonology.yaml` → `proto: hiatus_after`).
  The generator was making `*a.ar`, `*nei.va.o`; the roots only have hiatus
  in the `-ia/-io` endings. `*tuilar` 'hear' broke the rule and became
  `*tuliar` → `tulhar`. Modern hiatus stays free (`di.a`).
- **219 new roots** in 10 reviewed batches (special families + 200 core
  content words): body, nature, time, numbers 6–1000, animals, plants
  (new `plants.yaml`), people, home/food/objects (new `things.yaml`),
  religion (new `religion.yaml`), verbs, adjectives. All invented: the
  source languages (PT, FR, UA) steer only the sound, no borrowed shapes.
- **Sound change rules 5 and 6**, both found by `lnova gen` after the
  lexicon grew (more sound variety in the weights):
  - rule 5, `t c > ∅ / _ch`: `*pattia` → `patcha` → `pacha`. Rule 1 takes
    the second half of the geminate and **bleeds** rule 3.
  - rule 6, `p b c g f > ∅ / _lh`: `*plia` → `plha` → `lha`.
  A sweep of 877k legal generated proto words now gives 0 illegal modern
  words. No lexicon word changed.
- The `lnova gen` CLI test no longer pins exact words (they depend on the
  lexicon); `TestGen` still checks seed reproducibility.

### Key decisions (the user's choices)

| Area | Decision | Why |
|---|---|---|
| Hiatus | Proto: only after `i` | Matches the roots; rule 1 eats most of them |
| Inspiration | Sound flavour only | Keeps the a priori style of the first 104 roots |
| Sense families | Separate, unrelated roots | No derivational morphology until Phase 4 |
| Kill (6) | `hecar` kill, `grujer` murder, `talvir` execute, `sutar` slaughter/sacrifice, `lobiar` kill by accident, `zalder` kill in blood vengeance | Hebrew-style distinctions by intent and legality |
| Love (5 + 1) | `eluvir` agape, `ferzar` eros, `solhar` philia, `noidar` storge, `bauzer` love doing; `teuvar` now 'like' (things, not people) | Greek-style kinds of love |
| Create (4) | `eurir` from nothing (God only), `plodar` form/shape, `tecir` craft, `brener` build; `rucar` stays 'make/do' | Hebrew bara / yatsar / asah |
| God (4) | `Ouri` God (title noun with `lu`, capitalized in text), `nija` false god (f, 'a nothing'), `gulvo` idol, `zevo` deity | |
| Soul / spirit | `emia` soul (self), `ilso` spirit (life-breath, God's Spirit) | Hebrew nefesh / ruach |
| Kin pairs | Siblings and children share a root: `tirmo/tirma`, `nauso/nausa`; husband/wife separate | Romance -o/-a pairs |
| Modern `-ía` | Keep the stressed `-ía` (`ulvia`, `davia`), at most ~1 per batch | A flavour, not a pattern |
| Glosses | Multi-word glosses use Leipzig dots (`kill.by.accident`, `spring.water`) | Keeps interlinear glosses aligned |
| Near pairs | `eulir` 'sing' / `eurir` 'create' kept on purpose | Creation as song |
| Rules 5–6 | Repair rules at the end of the file, not reordering | Keeps earlier line numbers; later simplification is natural |

### Notes

- Rule hits over the 323 entries: rule 1 (palatals) 51, rule 2 (voicing)
  30, rule 3 (degemination) 27, rule 4 (final nasal) 9; 205 entries
  unchanged.
- The `-auja` trio `auja` morning, `lauja` illness, `gauja` beautiful is a
  side effect of `zi > j`; kept.
- **Profanity check** (PT, ES, FR, EN, UA, ~200 insults), by sound, not
  spelling (Rifatian `h` = /x/, so `hu-` sounds like ху-), over every
  inflected form (present, past, future, plural, possessive): 2367 forms.
  Changed: `bostar` 'carry' → `tolcar` (3sg was `bosta`, PT 'shit');
  `pulha` 'hen' → `dolha` (PT 'scoundrel'). Kept on purpose:
  `curvir` 'understand' (`curve` ~ UA курва), `sutar`/`rucar` (3sg
  `suta`/`ruca` ~ сука), `hersa` 'war' (~ хер), the `-loh` plurals
  (~ лох).

### Start next session here

1. `ROADMAP.md` → "▶ Next up": Phase 3, the dictionary web app.
2. Open question: re-seed the generator template weights from 323 roots?
3. Committed.

---

## 2026-10-16 (evening): Phase 2, step 2.6, word generator

### Done

- `phonology.yaml` → `proto: generator:`: weighted syllable counts and
  syllable templates per position (`mono`, `initial`, `medial`, `final`),
  seeded with the counts from the 104 roots. `phonology.md` → "Word
  generator" says the same in prose.
- `internal/phon/generator.go` parses and checks the block (template syntax
  `C{0,2}V{1,2}C?`, positive weights, the positions the counts need).
- `internal/gen`: `New(inv, corpus, rng)` + `Word()`. Picks a count, a
  template per syllable, then a sound per slot.
- `lnova gen [-n 20] [-seed N]`: keeps candidates that pass the proto
  phonotactics, aren't in the lexicon or the batch, and evolve into a legal
  modern word. Prints proto, modern, IPA, a homophone mark, and a summary
  with the seed and the rejection counts.
- Tests: generator block parse errors, seed reproducibility, a 5000-word
  distribution test (syllable shares, geminates only between vowels, ≥ 80%
  pass the proto check), `New` errors, CLI output.

### Result

Only about 10% of candidates are rejected (300 words: 24 proto phonotactics,
4 existing, 0 modern), so the templates and the validator agree. Rejects
come from what the templates can't see across a syllable boundary, e.g. an
accidental double consonant (`*al.la`: coda `l` + onset `l`, seen in tests)
or three vowels in a row; the breakdown by kind wasn't measured. **0 modern rejects**: every legal proto root evolves
into a legal modern word, a good sign for the rules.

First homophone found: `*ciu` → `chu`, the same as `*tiu` → `chu` (thou).
Rule 1 palatalizes both `ti` and `ci` before a vowel to `ch`, so the two
merge. Real languages do the same; it's the reason for homophones.

### Key decisions (the user's choices)

| Area | Decision | Why |
|---|---|---|
| Templates | **Per position** (mono / initial / medial / final) + weighted syllable count | Middles are nearly all `CV` and finals often a bare `V`; one flat list would produce shapes the roots never have |
| Template weights | **Hand-written in `phonology.yaml`**, seeded from the counts | `language/` is the source of truth; the user steers them |
| Sound weights | **Lexicon frequency per slot, +1 smoothing** | Skewed like real languages (r, a common; f, e rare); +1 keeps unseen legal sounds possible |
| Geminates | A medial coda may be `p t c`; the next onset is then the same consonant | "Geminate halves count as codas" from the last session; 10 roots have one |
| Homophones | **Shown, marked `= id`** | Merger is natural; the user decides whether to keep the word |
| Seed | Random by default, always printed | A good batch can be reproduced with `-seed` |

### Start next session here

1. `ROADMAP.md` → "▶ Next up": Phase 2 wrap-up, then Phase 3.
2. Committed.

---

## 2026-10-16 (later): proto phonotactics

### Done

- `phonology.yaml` got a `proto:` block (`codas`, `finals`, `geminates`).
  `phonology.md` → "Proto phonotactics" says the same in prose.
- `phon.CheckProtoPhonotactics`: the same per-syllable check as the modern
  one (both now share `check`), with the proto sets and geminates.
- `lnova validate` now runs the full proto check, not only proto-only
  sounds + `forbidden`. All 104 roots pass; `evolve` is still 104/104.
- **New rule, modern too: no double consonants.** A coda that repeats the
  next onset (`as.sa`) is an error unless it's a listed geminate. The modern
  language lists no geminates. `phonology.md` already said "no double
  consonants in modern words", but nothing enforced it: `assa` validated,
  because `s` is a legal coda and a legal onset. No current word is affected.
- Tests: a proto phonotactics table, Parse errors for the `proto:` block,
  validator cases (bad proto coda/final/geminate, modern double consonant).

### Key decisions (the user's choices)

| Area | Decision | Why |
|---|---|---|
| Proto codas | `r l s` (+ geminate halves) | Exactly what the 104 roots do; a modern coda `h` comes only from suffixes |
| Proto finals | vowel, `r n m` | Matches the data; rule 4 deletes n/m, giving vowel-final words with a nasal history |
| Geminates | `pp tt cc`, only between vowels | Rule 3 degeminates only those; anything else would leak a double consonant into modern |

Side effect: `*apta`-type clusters (stop + stop, Latin *nocte*) are illegal
in proto. That follows from codas `r l s`.

### Start next session here

1. `ROADMAP.md` → "▶ Next up": step 2.6, the generator. Decide the syllable
   templates and weights first.
2. Committed.

---

## 2026-10-16: Phase 2, step 2.5, rule engine + `lnova evolve`

### Done

- `internal/sound/evolve.go`: `Evolve(inv, rules, word)` runs the rules in
  file order, each on the previous output, and returns the final form plus
  a `[]Step{Rule, Before, After}` derivation. Only rules that changed the
  word leave a step.
- `lnova evolve`: every proto form against `modern`, compiler-style
  mismatch lines (`file:line: id: *proto → got, want modern`), summary
  `104 entries, 0 mismatches`, exit 1 on any mismatch.
- `lnova evolve -trace <id|*proto>...`: one line per rule that fired
  (`28   si > x / _V   esia → exa`). A `*` argument is an ad-hoc proto form
  not in the lexicon; a lexicon entry that doesn't reach `modern` gets a
  `want` line and exit 1.
- Tests: an `Evolve` table (feeding and bleeding cases), the ordering test
  (`patta` → `pata` vs. `pada`), the whole lexicon as a table test
  (`TestLexiconEvolves`, one subtest per id), and CLI tests, including a
  temp lexicon with a wrong `modern`.

### Result

**104/104 entries match.** The engine and the hand-written `modern` forms
agree, so no data or rule disagreements came up.

Experiment (temp copy, `language/` untouched): swapping rule groups 2
(voicing) and 3 (degemination) breaks 10 words, e.g. `*pippo` → `pibo`,
`*crotta` → `croda`, `*ruccar` → `rugar`. That's degemination *feeding*
voicing, the reason the file orders them the way it does.

### Key decisions

| Area | Decision | Why |
|---|---|---|
| Trace content | Only rules that changed the word | A trace of 14 lines with 13 no-ops hides the story; `lnova rules` lists all of them |
| Ad-hoc forms | `-trace '*form'` evolves any proto string | Lets you try a word before adding it, and is what 2.6 needs |
| Mismatch line | Points at the `modern:` line | That's the line you'd edit if the data is wrong |

### Start next session here

1. `ROADMAP.md` → "▶ Next up": decide the proto phonotactics, then 2.6.
2. Step 2.5 is **not committed yet**.

---

## 2026-10-15 (night): Phase 2, step 2.4, sound change rule parser

### Done

- `internal/sound`: `Parse`/`Load` turn `sound-changes.txt` into `[]Rule`
  (target, replacement, left/right environment, line, source text). Items
  are tokenized by greedy longest match over segment spellings and category
  names, so `V_V` and `_#` need no spaces. Every bad line is reported as
  `file:line: message` (an `ErrorList`); good lines still parse.
- `Rule.Apply(inv, word)`: one rule, simultaneous application. Ordered
  application of all rules and tracing come in 2.5.
- `phon.Matches` exported (was `matches`) for category matching.
- `lnova rules` lists the 14 rules with line numbers; exit 1 on parse errors.
- `sound-changes.txt`: only the header comment changed (settled syntax and
  semantics); the rules are untouched.
- Tests: parse table, one error case per check, error collection, the real
  file (14 rules, line numbers), an apply table, the semantics test, and the CLI.

### Key decisions (the user's choices)

| Area | Decision | Why |
|---|---|---|
| Application | **Simultaneous**: match against the form as it was before the rule, non-overlapping, left to right | Standard (SPE) model, predictable; an iterative flag can be added if a spreading rule (harmony) ever needs it. No current word differs between the two |
| Rule identity | **Line number + text**; `# 1.` headings stay plain comments | No format change. Cost: editing the header shifts the numbers (it did this session, 17 → 26), so tests that pin line numbers need updating |
| Syntax | Only what the file uses: segments, categories, `_`, `#`, `∅`, optional environment | Add sets `{p,t,c}` or optional `(C)` when a real rule needs them |
| Comments | Full-line only; a line starting with `#` is a comment | `#` inside a rule is the word boundary. Side effect: a rule can't start with `#`, which is fine because targets never contain `#` |

### Start next session here

1. `ROADMAP.md` → "▶ Next up": step 2.5, rule engine + `lnova evolve`.
2. Step 2.4 is **not committed yet** (2.1–2.3 are, in `b7eae8e`).

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
