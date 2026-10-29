# Session log

Newest first. One entry per session: what was done, what was decided and
why, and where to pick up. The task list itself lives in `ROADMAP.md` →
"Next up"; this log gives the context behind it.

## 2026-10-29: Retrieval that scales to 300k entries

### Why

The lexicon is meant to grow to ~300k entries (generated, hand-added,
maybe imported later). `-ctx full` puts every entry in the prompt, and
`-ctx retrieved` only looked scalable: it took *every* entry linked to
a word. Decided (the user): pure Go with our own index, hybrid retrieval
(tagger + BM25 + dense) fused with Reciprocal Rank Fusion, a per-word
limit and a token budget, `full` kept but refused when it doesn't fit.

### Done

- `lnova fixture [-n 300000] [-senses 0.1] [-homonyms 0.01]`: a copy of
  `language/` in `.cache/fixture-300k` with 300k synthetic entries
  (`internal/fixture`): 10% new senses of real words (`water.cold`), 1%
  exact homonyms (`water` with another meaning), the rest made-up glosses
  with definitions built from real words. Synthetic files load first, so
  ties in lexicon order go against the real entries (worst case).
- `lnova eval -retrieval [-gold language]`: the retriever alone, no model.
  Gold = the entries of the rule-based reference translation (from the
  real lexicon with `-gold`, since on the fixture the reference itself
  uses synthetic senses). Reports recall, entries, tokens, time. A test
  holds the real lexicon at 100%.
- `internal/retrieve`: BM25F index (gloss ×3, id, definition; S-stemmer;
  stopwords), and a `Retriever` that fuses per English word:
  1. the tagger's readings, ranked in **tiers** (exact gloss, then senses;
     the tagger's chosen category first), each tier a tied rank;
  2. BM25 on the lemma (equal scores tied);
  3. simplified **Lesk**: BM25 of the word + the sentence's other content
     words, among the word's own entries (sense disambiguation);
  4. with `-dense`, embeddings.
  RRF (k=60), ties broken by BM25 score, top `-k 3` per word, `-budget
  2048` tokens over all words (each word keeps its best entry). Closed
  classes are always in, as before. `lookup` (tools) uses it too, 5 per word.
- `internal/semantic`: int8 vectors + sign bits; search = Hamming first
  pass (≥400 candidates) then int8 rescore. Binary append-only cache
  `.cache/embeddings-<model>.bin` (every batch of 256 synced: an
  interrupted 300k build resumes; compacted when records go stale); the
  old JSON cache is imported once (6 MB → 0.5 MB). `-dims` for Matryoshka
  truncation, off by default (see below). `lnova llm|eval -dense`.
- `Translate` refuses a prompt over `MaxNumCtx` (32K): `-ctx full` on a
  big lexicon errors instead of being silently cut.
- Startup: `loadLLM` parsed the lexicon twice (`translate.LoadWith`
  fixes it), and `lexicon.LoadCached` keeps a gob snapshot in `.cache/`
  keyed by a hash of the files (YAML 3.2 s → gob 0.15 s at 300k).
  `lnova validate` still parses YAML (field line numbers).
- Eval results record the retrieval settings (`meta.retrieval`); `-resume`
  refuses to mix runs of different retrievers.

### Numbers (31 eval sentences)

| | real (631) | fixture 300k | before, fixture |
|---|---|---|---|
| recall | 100% | 99.2% | 100% (everything in) |
| entries per prompt | 29 | 33 | 927 |
| tokens per prompt | 7154 | 7246 | 28827 |
| context build | 1 ms | ≤ 43 ms | ≤ 974 ms |
| `loadLLM` (warm) | 0.1 s | 1.6 s, 450 MB | 7.6 s, 940 MB |

The harsh fixture (`-homonyms 0.45`: ~170 exact "water"s) also gives
99.2%. ~6.9K of the ~7.2K tokens are the fixed part (grammar,
morphology, examples, closed classes). Dense at 300k × 768 (random
vectors): 260 MB, 5.7 ms per query (exact int8 scan: 298 ms).

Offline, on the real 768-d vectors (every entry as a query, recall@10
vs float32): int8 99.3%; truncation to 512 dims 84%, 256 dims 73%;
sign bits alone 58%, sign bits (200) + int8 rescore 99.1%.

### The linguistics / AI

- **RRF needs real rankings.** A channel's order must mean something: the
  tagger's list for "water" was 383 entries in file order, and fusing
  that as a ranking let file order choose (recall 43–50%). Ranking it in
  tiers (tied ranks) and letting BM25 break the ties gave 97%; a
  tie-break on BM25's score and case-blind gloss matching (`God`) 99%.
- **The fixture can lie.** The first fixture gave ~300 synthetic entries
  glossed exactly "sleep" (WordNet: ~8 senses). No lexical signal can
  separate 300 identical glosses, so it measured the fixture, not the
  retriever. The homonym share is now explicit.
- **Lesk (1986)**: the sense whose definition shares words with the
  sentence wins (`love.agape` says "God's love"). BM25 does the
  counting, so a shared rare word counts more.
- **Quantization is nearly free, truncation isn't**: int8 keeps 99% of
  the neighbours; nomic's Matryoshka 256-d prefix only 73% here.

### Not done / found

- **Dense not measured with the real model**: Ollama wasn't running.
  Without it, `-dense` warns and the contexts stay lexical.
- The one miss: "We will love the god." → `love_agape`. On the fixture,
  three synthetic verbs glossed exactly "love" take the 3 slots
  (tagger tier 0 + BM25 rank 0 beat Lesk's one vote). Not tuned away:
  weighting channels on 31 sentences would be overfitting.
- **The rule-based translator doesn't scale yet**: `Checker.Expect` (the
  meaning check, run for every `lnova llm`) takes 2.8 s per sentence at
  300k, against 0.15 ms now; it tries every sense combination (time in
  `Analyzer.Analyze` and GC). It also prefers synthetic senses. Out of
  scope today.

### Open (for the user)

1. Run with the model: `lnova similar -eval evals/similar.yaml` with
   `-dims 768/512/256` (real recall, not neighbour agreement), and
   `lnova eval -retrieval -dense`. Make `-dense` the default?
2. The unseen eval (2026-10-28) is 22/45 and was made with the old
   retriever: finish it on commit 66ed39b (`-resume` now refuses to mix),
   or rerun it with the new one as the baseline.
3. Scaling the rule-based translator / meaning check (above).

### Start next session here

ROADMAP → Next up.

---

## 2026-10-27: Eval baseline for the pronoun-after-preposition sentences

### Done

- New eval tag `pp-pronoun` on the 2 sentences from 10-25 (`-tag
  preposition` also picks up the 3 `frame` sentences that already have a
  baseline). `lnova eval -tag pp-pronoun` (defaults: 3 setups × 5 runs,
  temp 0.7, qwen2.5:7b), saved as
  `evals/results/2026-10-27-pp-pronoun.json`, the baseline for these 2
  from now on.

| sentence | retrieved | retrieved+tools | none+tools |
|---|---|---|---|
| She sleeps with him. | 5/5 | 5/5 | 5/5 |
| The woman gave bread to me. | 4/5 | 2/5 | 4/5 |

Median time per translation: 26 s / 65 s / 36 s. Tool calls per
translation: 0 / 12.4 / 5.3 (46 and 21 repeated calls in all).

### Failures (5, all "gave bread to me")

1. **Wrong tense / made-up form** (2): `hibare` (future) for "gave",
   `hibarexe` (future + past stuck together, and `gurna` dropped).
2. **Bread made plural** (3): `gurnah`, in `gurnah sela mi` (`sela`
   for `epe`), `mi epe gurnah` (object and PP swapped) and `gurnah mi`
   (no `epe`: an English-style double object, "gave me bread").

The model **never** used the subject form after a preposition (`gu
hana`, `epe mu`): 0 of 30. The pronoun rule itself caused no failures.

### The linguistics / AI

- **Test contamination.** Both sentences, with their exact answers, are
  in grammar.md (worked example 4, and the `epe` paragraph), and the
  whole grammar goes into every prompt. So this baseline mostly measures
  **copying from the prompt**, not applying the rule. In ML terms the
  test set leaked into the training/context data, and scores on leaked
  items are inflated. That makes the 5 failures more telling: with the
  answer in front of it, the model still changed `hibaxe` and `gurna`.
- **Double object vs. PP.** English has two frames for "give": *gave
  bread to me* and *gave me bread* (the **dative alternation**). `gurnah
  mi` looks like the model mixing in the second frame. Rifatian has only
  the PP frame (S V O PP), as far as grammar.md says.
- **Tools again cost more and helped less**: retrieved+tools was the
  slowest and the worst (2/5), and both of its long runs (165 s, 192 s)
  failed. Same pattern as 10-23. With 5 runs the intervals are ±25
  points, so only "the pronoun is never wrong" is a solid result.

### Open (for the user)

1. **Make the pronoun test unseen?** E.g. add "He sleeps with her." /
   "The man gave water to us." (or replace the two), so the eval
   measures the rule and not copying. Would need entries for every word.
2. **Dative alternation**: is "The woman gave me bread." (no "to") valid
   English input, and does it translate to the same PP sentence? Today
   the English grammar has no double-object VP.

### Start next session here

ROADMAP → Next up, and the two questions above.

---

## 2026-10-25: Pronouns after prepositions take the object form

### Decided (the user)

- **A pronoun after a preposition is in the object form**, as in
  English: `Hana sile gu honi.` = "She sleeps with him.", `epe mi` "to
  me". No new forms. Other options were the subject form (Portuguese
  *com ele*) and fused forms (Portuguese *comigo*, Irish *liom* "with
  me"). Whether a PP can come first (`Bi lu hoida, …`) is still open.

### Done

- grammar.md: the rule in Pronouns and Prepositions, worked example 4
  (`Hana sile gu honi.`; the examples are also the LLM prompt's worked
  examples), open question narrowed.
- Translator (`internal/translate/generate.go` `pp`): no more refusal.
  The NP after a preposition was already built with `OBJ`.
- Glosser (`internal/gloss/gloss.go` rule 2): a word right after a
  preposition must not be in the subject form ("SBJ after a
  preposition"), and that check replaces the position checks. Without
  it, `Lu dauzo iha gu hani` ("The king is with her") got "OBJ after the
  copula", because that rule looked at every word after `ihar`.
- Tests: 3 translator rows, 3 glosser rows. `language/evals.yaml`: "She
  sleeps with him.", "The woman gave bread to me." (no baseline yet;
  not tagged `frame`, which stays the 6 sentences of the 10-22 baseline).

### The linguistics

- **Oblique case.** Languages that split "subject" from "everything
  else" have an *oblique* form for the else: objects, and pronouns after
  prepositions. English is like this (*he* vs *him*, both "see him" and
  "with him"), and so is Rifatian now: `-i` was called "object" but is
  really oblique. Portuguese split it three ways for 1st/2nd person:
  *eu* (subject), *me* (object clitic), *mim* (after a preposition), and
  it fused *com + migo* from Latin *mecum*.
- **Why case survives on pronouns.** English and the Romance languages
  lost case on nouns but kept it on pronouns. Pronouns are frequent and
  short, so they resist regularizing, the same reason irregular verbs
  are the common ones.

### Start next session here

ROADMAP → Next up. Running `lnova eval -tag preposition` once (needs
Ollama) gives the 2 new sentences a baseline.

---

---

## 2026-10-23: Eval re-run on the frame-check sentences

### Done

- `lnova eval -tag preposition` and `-tag copula` (defaults: 3 setups ×
  5 runs, temp 0.7, qwen2.5:7b), code `ea0b5dd`. Together they cover the
  5 sentences that moved to the frame check, plus "The king was a
  warrior.", which already had it. Saved as
  `evals/results/2026-10-22-frame-prep.json` and `…-frame-copula.json`.
  These are the baseline for these 6 sentences from now on.

| sentence | retrieved | retrieved+tools | none+tools |
|---|---|---|---|
| The children of the king sleep in the house. | 0/5 | 2/5 | 2/5 |
| The birds fly over the mountain. | 1/5 | 0/5 | 0/5 |
| The woman gave bread to the child. | 3/5 | 3/5 | 2/5 |
| The king was a warrior. | 1/5 | 1/5 | 0/5 |
| The warrior's sword is long. | 2/5 | 4/5 | 5/5 |
| I think that the river is cold. | 5/5 | 5/5 | 1/5 |

Prepositions 13/45 (29%), copula 24/45 (53%). Median time per
translation: 24 s / 85 s / 137 s (prep), 14 s / 45 s / 103 s (copula).
The comparison with the 10-18 baseline means little: that partial run
has only 1–2 translations of each sentence per setup.

### Failures, by kind (53 in all; some have more than one)

1. **Invented word forms** (~17): `frinah` for `frinhah` "birds" (6×),
   `orlah`, `silah`, `silih`, `gurza`, `dauzah`, `hido`.
2. **Dropped article** (~13): `ihaxe gurzo` without `ne` (9 of the 13
   "king was a warrior" failures), `fai velda`, `epe orlo` without `lu`.
3. **Plural subject, singular verb** (6): `frinhah leta` instead of
   `letah`.
4. **Infinitive instead of a conjugated verb** (5): `letar`, `ihar`,
   `silir`, sometimes moved to the end of the clause (S O V).
5. **Wrong tense** (4): `hibare` (future) for "gave", `iha` (present) for
   "was".
6. **Correct sentence, wrong word list** (1): `Mu sozu itu lu trubo iha
   grepo.` is exactly the reference, but the model's `words` list gave
   `itu`, `lu`, `trubo`, `grepo` the id `i`, so the checker failed it.
7. Nonsense (2): `epe selah trubo hurito sela orlo`, `menah truboh`.

### The linguistics / AI

- **Why `frinhah` breaks.** The plural adds `-h` to `frinha`, and `nh`
  is one sound written with two letters. The model doesn't see sounds or
  even letters: it sees **subword tokens** (chunks like `fr`, `inh`,
  `ah`), so `frinhah` with two `h`s looks like a typo to "fix". Rules
  that are simple over segments (add `-h`) can be hard over tokens. This
  is the same reason LLMs miscount the letters in a word.
- **Dropping `ne` follows Portuguese.** *O rei era guerreiro*: Romance
  languages (and many others) leave out the indefinite article before a
  predicate noun, especially for professions and roles. grammar.md cites
  Portuguese for the copula, and the model's Romance-like habits match.
  Rifatian says articles are "used like English", so `ne` is required
  and these fail. That's a design choice, so it's listed below.
- **Tools cost a lot and helped little.** On prepositions, the lookups
  (15–33 per translation, most of them repeated) took 4–6× the time for
  27% → 33%. They help where the error is a missing word (the sword: 2/5 →
  4/5), not where it's a wrong form the model is sure of (`frinah`).
- **Noise.** With 5 runs, a 95% interval is ±20–25 points. Only big
  gaps mean anything: "I think…" 5/5 vs 1/5 without retrieval, and the
  birds being near 0 everywhere.

### Decided (the user)

1. **`ne` before a predicate noun stays required**, as grammar.md says
   (articles "used like English"). `Lu dauzo ihaxe gurzo.` stays a fail.
2. **A correct sentence with a wrong word list passes.** In `Verify`
   (`internal/llm/check.go`), a word claimed as entry X that the analyzer
   reads only as entry Y is now a **remark**, like a wrong feature or a
   made-up id. The meaning check still catches a wrong *word* (`Lu huro
   nara…` for "The woman sees…"). The saved copula results predate this:
   re-checked, "I think…" none+tools is 2/5, not 1/5 (copula 25/45).
3. **Tag `frame`** in `language/evals.yaml` on the 6 sentences:
   `lnova eval -tag frame` runs them all at once.

### Start next session here

ROADMAP → Next up.

---

## 2026-10-22 (English side): copula, prepositions, "of", `itu` clauses

### Done

- **`words.yaml`**: new categories `Be` (am/is/are/art/was/were/wast/wert,
  bare `be`, each with its tense), `P` (in/at → `bi`, to → `epe`, with →
  `gu`, over/above/across → `fai`), `Of`, and `Comp` (a second reading of
  "that" → `that_comp`, `itu`). The forms of be left `present:`/`past:`;
  `has`/`hath` → have went in (they were `?has` before).
- **`grammar.cfg`**: `Pred -> Be Compl | Be Neg Compl | Aux Be Compl …`,
  `Compl -> Adj | NP | PP`, `S -> Be NP Compl` (Is the river cold?),
  `VP -> V PPs | V NP PPs | V Sub`, `PP -> P NP`, `Sub -> Comp NP Pred |
  NP Pred`, `NP -> NP Of NP`.
- **Transfer** (`internal/translate/generate.go`): `clause` is recursive.
  A `Sub` becomes its own clause after `itu`, and `itu` is inserted when
  English dropped "that". PPs come after the object (S V O PP). `X of Y`
  is generated as `Y's X`. When the translator gives no translation, the
  error now says why.
- **Ranking**: a translation with no unknown words now comes before one
  with an unknown word, whatever the tagger thinks. The tagger reads
  "cold" in "Is the river cold?" as a noun (`?cold`); the adjective
  reading gives `grepo`.
- **Glosser**: after `ihar`, a pronoun must be in the subject form (your
  decision); `Hunu iha mi` gets "mi is OBJ after the copula".
- **Eval**: the 5 former `no-parse` sentences now get the **frame check**
  (1 reference each); the tag is gone. The lexical check counts `Be` and
  `P` as content words, so a dropped `ihar` or preposition still fails
  it when a sentence doesn't parse.

### The linguistics

- **Why `be` is its own category.** In English, *be* is the one verb
  that inverts without do-support (*Is it cold?*, not *Does it be cold?*)
  and takes an adjective as its complement. Grammars of English give it
  a category of its own for that reason. Rifatian `ihar` is an ordinary
  class I verb, so the special handling stays on the English side.
- **PP attachment.** "I saw the man with the telescope" has two trees:
  the PP modifies *saw* or *man*. This is the textbook ambiguity of
  parsing, and an Earley parser returns both trees. Here PPs attach only
  to the verb phrase. A PP after a noun would give the same Rifatian
  string (the PP follows the object either way), so the extra trees
  would only multiply the work. Sentences like "The dog in the house
  sleeps" don't parse and fall back to the lexical check.
  `NP Of NP` has the same kind of ambiguity:
  "the house of the woman's dog" is [house of [woman's dog]] or
  [[house of woman]'s dog], and both translations are listed.
- **Complementizer insertion.** English may leave out "that"; Rifatian
  never leaves out `itu`. So the transfer step adds a word that is
  absent from the source, like `na` for double negation.

### Decided (the user)

1. A pronoun after `ihar` is in the **subject** form: `Hunu iha mu` "It
   is I" (grammar.md, Copula).
2. "a/this X of Y": **no translation** for now; added to grammar.md →
   Open questions. Only "the X of Y" and "X of Y" translate.

Pronouns after prepositions stay undecided, so "She sleeps with him"
gets no translation ("him: a pronoun after a preposition isn't decided
yet").

### Known limits

- English agreement isn't checked: "The children of the king's dog
  sleep" lists "the dog sleeps" (`… huro sile`) first, though plural
  *sleep* says the children are the subject.
- The frame check puts PPs and copula complements in the Clause's
  `Object` slot, so a missing `fai` is reported as "object: the English
  needs "over the mountain"". It catches the error; the slot name is
  just broad.
- Not re-measured: Ollama wasn't running.

### Start next session here

ROADMAP → Next up (#1: re-run the eval on these 5 sentences).

---

## 2026-10-22 (later): Semantic search with embeddings

### Done

- **`POST /embed`** in `llm/app.py`: texts → vectors via Ollama's
  `nomic-embed-text`. That model was trained with a **task prefix** per
  side of a search (`search_query: ` for the short query, `search_document: `
  for what is searched), so the service adds it; other models get the
  text as is. The error mapping of `chat()` is shared now (`call()`).
- **`internal/semantic`**: one vector per entry, built from
  `gloss (pos): definition` (your choice), scaled to length 1 so cosine
  similarity is a dot product. Search is a brute-force scan (631 × 768
  floats, under a millisecond); vector databases pay off at millions of
  documents. Vectors are cached in `.cache/embeddings-<model>.json`
  (gitignored, outside `language/`), keyed by a hash of each entry's text:
  an edit re-embeds only that entry. First build 44 s (model load plus 631
  texts), then about 1.5 s.
- **`lnova similar [-k 10] [-pos P] <english>`**, and **`-eval
  evals/similar.yaml`**: 12 queries with the ids a good search should
  return; the score is **recall@10** (the share of the expected ids in the
  top 10). Precision isn't scored, because the lists are short, not
  complete.
- **Web**: under the search results, an htmx fragment (`GET /similar`,
  `hx-trigger="load"`) lists "Related in meaning", so string search stays
  instant. The index is built on the first such request (the service may
  start after the server); without the service it's one muted line.
  Flags `-llm URL` ("" = off), `-embed-model`, `-cache`.
- **`lnova coin -related 10`** (your choice: 10 by meaning + 20 spread):
  the concept is embedded like an entry, the 10 nearest roots (no derived
  words, any part of speech) come first among the 30 examples, and
  stderr prints them. If embedding fails, coin warns and goes on with the
  old examples.

### First numbers (nomic-embed-text)

- **recall@10 = 62%.** Good: killing 6/6, love 5/5, weather 5/6,
  religion 6/8. Bad: "a large wild cat" 1/8 (jaguar only; lion, tiger…
  missing), "birds of prey" 1/8 (osprey only).
- Why it misses: most definitions are thin ("A lion.", "An eagle."), so
  "lion is a big cat" has to come from the embedding model's world
  knowledge, and a 137M-parameter model has little. Disambiguated glosses
  (`kite.bird`, `swift.bird`) embed as "kite bird", which pulls every bird
  together: "birds of prey" finds kite, swift, swallow, crow, turkey.
- **Experiment (not kept): adding the lexicon file as a category**
  (`lion (noun, mammals): A lion.`) gives 61%, no better: birds of prey
  12% → 38%, sea 57% → 43%, farming 80% → 60%. The script (Python against
  `/embed`) reproduced Go's 62% for the baseline text, so the two agree.
- **Coin, lantern** (seed 1): related examples eiva (light), voixa (moon),
  eiro (sun), and then trusa (squid), sersebo (lion), vausove (whale),
  maldo (frog)… After `light` (0.64) every score is about 0.55: the
  lexicon has few words about lamps, so ranks 2–10 are just the least
  unrelated entries. A query with a definition doesn't fix it. Roots
  with related examples: *mira *fela *nira *sila *tira (100% accepted,
  CV.CVa as before); without: *zilpa *lirfa *pirva *firsa *girfa. One run
  each, so this shows nothing either way about quality.

### Decided (the user)

1. **Coin's related examples get a threshold**: `-related-margin 0.05`
   keeps only roots within 0.05 of the best score (`CoinOptions.RelatedMargin`;
   0 = keep all N). A margin, not a fixed cut-off, because scores shift
   with the model and the query. Live (the concept embedded as
   `english (pos)`):
   - ocean: sea 0.78, squid 0.73 → 2 kept (dolphin 0.73 just out);
   - tiger cub: tiger → 1 kept (cub is derived, left out);
   - whisper: breathe, say, push, sing, hear, swim, voice, smell → 8 kept;
   - lantern: light 0.72, then squid, moon, leaf, lion, whale 0.67–0.69 → 6
     kept. Thin entries ("squid (noun): A squid.") cluster together, and
     no margin separates them; richer definitions would.
2. **`evals/similar.yaml` is fine as written.**

Still open: richer definitions would help search more than any text
format (a lexicon edit, so yours).

### Start next session here

ROADMAP → Next up.

---

## 2026-10-22: Coining new words

### Done

- **`lnova coin`** (`internal/llm/coin.go`, `POST /coin`, prompts
  `llm/prompts/coin_*.j2`). Your decisions:
  - the model invents the **proto** roots;
  - fresh roots only, no derivations;
  - output is YAML printed for you to paste (the lexicon is never written);
  - homophones are rejected and sent back as feedback.

  Go checks each candidate in stages:
  1. sounds (proto only);
  2. proto phonotactics and the verb ending;
  3. `sound.Evolve` gives the modern form, which must pass the modern
     phonotactics;
  4. the root must not exist yet, and the modern form must not be a
     homophone of an existing word or of an earlier candidate;
  5. `lexicon.Validate` on the entry it would become.

  Rejections are counted by kind, and a repeated candidate counts as
  `repeat`. Gender/ending mismatches (an f noun in -o) are remarks, not
  rejections, because grammar.md says "typically". It refuses a concept the
  lexicon already has, unless you pass `-id`.
- Prompt: the phonology is written out from `phonology.yaml`: sounds,
  clusters, codas, finals, forbidden sequences, and template percentages
  from the generator block. It also gets 30 example roots: same pos (and
  gender), spread over the lexicon, with derived words (notes with "+") left
  out.
- `sound.Rule.Group`: the "# N." heading number, so coined notes read
  `(rule 2: c > g / V_V; rule 3: tt > t)` like the hand-written ones.
- Shared HTTP code in `Client.post`. On the Python side, a shared `chat()`
  helper does the error mapping.

### First numbers (qwen2.5 7B, temp 0.7, 6 concepts × 2 seeds)

- **79% of candidates accepted** (55/70); 78% in round 1. `lnova gen`'s
  random roots: 87% (500/578).
- Nouns and adjectives: nearly 100%. Verbs are harder: whisper got 40–70%,
  with illegal onsets `sl zl zr`, hiatus `u.a` and coda `v`.
- **Typical failures: English habits.** The model writes `k` and `q`
  (*kilir, *squil, *tiroko) although the prompt says `c` = /k/. It reaches
  for English clusters (*slerar). It copied example roots twice (*gilcar
  "laugh", *bralar "dance") and repeated a rejected candidate.
- **The accepted roots are samey.** Before the prompt fix, all five lantern
  roots were *l*-initial (the English word's first letter) CV.CVa. After
  it, the first letters vary, but the shapes are still mostly CV.CVa.
  Verbs cluster on -irl-: *birlar, *dirlir, *zirlar. The model's own
  "reasons" are wrong about syllable shapes (it called *zula* "VCCV"): it
  can't see phonemes, only tokens.
- The model first misread "definition" as a description of its roots. The
  user prompt now asks for an English dictionary definition, and it writes
  real ones.
- No homophone rejections in these runs. Random `gen` roots hit them often,
  because short CV words collide (*von → vo "and").

### Decided (the user, after the first runs)

1. **Sameness: accepted.** You pick from the list anyway; no variety tricks.
2. **`k`/`q` get a hint:** a candidate with k or q is rejected with
   "unknown letter 'k' … (write /k/ as c)", so the feedback says what to
   do instead.
3. **Meaning-aware examples: next session**, with semantic search
   (ROADMAP → Next up).

### Start next session here

ROADMAP → Next up.

---

## 2026-10-18/19: Evals, and the grammar they needed

### Done

- **`lnova eval`** (`internal/eval`): `language/evals.yaml` (26 tagged
  sentences) × setups × runs; run *i* uses seed *i*. Per setup: pass rate
  and first-attempt rate with **Wilson 95% intervals**, errors, median
  time and tokens, tool calls. `-out` saves after every translation,
  `-resume` continues an unfinished file, `-baseline` diffs two runs,
  `-show` reports a saved file, `-list` checks the set without the model.
- **Temperature + seed** through Go → service → Ollama. Every call was
  at temperature 0 (greedy), so repeated runs would have repeated
  themselves. `lnova llm -temp 0.7 -seed 3` replays an eval run.
- **Meaning-check bugs found by `-list`**: prepositions fell through to a
  fuzzy search (`of → shore, cub`) and `is` didn't lemmatize to `be`, so
  the lexical check demanded nonsense entries. Now: words with no
  category count only if a gloss matches exactly (in → `bi`, over →
  `fai`), definitions are never searched, and `words.yaml` has
  `present:` (am/is/are/art → be) and was/were/wast/wert → be.
- **Grammar (the user's decisions, grammar.md)**: copula always `ihar`;
  prepositions before the noun, PP after the object (S V O PP), no
  contractions; no "of" (X of Y = Y's X); subordinate clauses with
  `itu` (from "what", as Portuguese *que*), obligatory. New entries:
  `that_comp` (`itu`, pos `complementizer`, gloss COMP) and `over`
  (`*fai`, from `lnova gen`). Glosser: a CG rule reads `itu` as COMP after
  the clause's verb (a clause boundary) and as "what" elsewhere;
  `gloss.Clause.Comp`.

### Baseline (partial, the user's choice): 133 translations

`evals/results/2026-10-18-qwen2.5-7b.json`; `lnova eval -show` on it.
Temperature 0.7, 3 attempts. All of run 1, most of run 2; none+tools has
only run 1 for most sentences.

| | retrieved | retrieved+tools | none+tools |
|---|---|---|---|
| pass rate | 35% [23–48] (18/52) | 44% [32–58] (23/52) | 48% [31–66] (14/29) |
| first attempt | 15% | 29% | 31% |
| median time | 21 s | 66 s | 78 s |
| median prompt tokens | 21K | 70K | 72K |
| tool calls / translation | 0 | 10.2 (176 repeated) | 16.8 (152 repeated) |

- **The intervals overlap: no setup is shown to be better.** Last
  session's 3/7 vs 6/7 was one greedy run; with sampling and more
  sentences the gap mostly disappears.
- Much lower than last session's ~6/7. Likely temperature 0.7 (vs 0)
  plus harder sentences. Untested: a temp-0 run would tell.
- Never passed in any setup: `We will love the god`, `The old king built a
  fortress`, `Did the farmer sell the wine?`, `Why did the child run?`,
  `My mother sings`, `The shepherd carried the small child`, `The friends
  hear the thunder`, `Who will help the old man?`.
- Tools: ~⅓ of all calls are repeats (dedupe answers them). The model
  loops instead of using the result.
- New grammar works from the prompt alone: prepositions 2/2 with
  retrieved+tools; `Mu sozu itu lu trubo iha grepo` passes.

### What went wrong

- First attempt (47 translations) lost: the session ended while the eval
  was paused, and it only saved at the end → save-per-translation and
  `-resume`; detached with `setsid nohup`.
- Second attempt: **Ollama hung** ("Stopping…" in `ollama ps`) at
  translation 134; every later one hit the 5-minute timeout. Cause:
  memory. 14/15 GB used, load ~30, `llama-server` 85% GPU / 15% CPU
  offload, plus Zoom etc. Killing the stuck `llama-server` freed it.
  The 23 timeout records were dropped from the baseline.

### Start next session here

1. Decide: finish the baseline (`-resume` needs a `-retry-errors`
   flag or the file as is) on a quieter machine, and/or a temperature-0
   run for comparison.
2. Coin new words (ROADMAP → Next up).

---

## 2026-10-18 (evening): Tool-use measurement, finished

### Done

- All three columns rerun on the **629-entry** lexicon (the user's choice,
  so every number comes from the same data): 7 sentences ×
  `-ctx retrieved`, `-ctx retrieved -tools`, `-ctx none -tools`, one run each,
  all with `-compare`. Script and transcripts are in the session scratchpad
  (not in the repo); Evals will turn it into a real tool.
- No code changes.

### Measurements (629 entries; one run each)

| sentence | retrieved | retrieved + tools | none + tools |
|---|---|---|---|
| The woman sees the river. | ✓ | ✓ (1 call) | ✓ (7) |
| The men will drink the water. | ✓ | ✓ (5) | ✓ (11) |
| Didst thou see the woman's dog? | ✓ | ✓ (1) | ✓ (1) |
| The dog eats the bread. | ✗ `baugir` ×3 | ✗ 120 calls, 423 s | ✗ `baugir`/`baugiru` (26) |
| She didn't see the big house. | ✗ `nara`, `gumo hoida` | ✓ (5)¹ | ✓ (15) |
| Who saw the men? | ✗ `naraxis` ×3 | ✓ (3) | ✓ 2nd attempt (7) |
| We will love the god. | ✗ `bauzareh` | ✓ `noidaremu` (2)² | ✓ `Eluviremu`, pro-drop (8) |
| **right** | **3/7** | **6/7** | **6/7** |
| time per sentence | 6–17 s | 11–29 s (423 s doom loop) | 17–126 s |
| prompt tokens per sentence | 6K (1 attempt) – 18K (3) | 19K–47K (139K) | 17K–112K |
| `lookup` calls | n/a | 1 in total | 18 in total |

¹ The first try of this cell stalled past the 10-minute client timeout
(`ollama ps`: "Stopping..."). Ollama answered again right after, and the
rerun took 27 s. So the hang doesn't reproduce consistently; it's one request
that never finishes, not a dead server. The 512-token tool-round cap was hit
5 times with an empty reply (the model produced no parseable call).

² `noidar` is *love.storge* (family love). It's one of the 5 references,
because the rule-based translator offers every `love_*` verb, so the meaning
check accepts it, but it's the wrong kind of love for a god. The check
accepts any translation that's a correct sense of the word, not only the
best one.

Compared with 323 entries: retrieved stays 3/7 (same failures);
retrieved+tools goes 5/6 → 6/7 (the god sentence is now measured and passes).

### What the transcripts show

- **`The dog eats the bread.` fails in every setup.** The model is sure the
  verb is `baugir` and keeps it, whatever `check` and `inflect` say. With
  retrieved+tools it doom-looped: `check("Lu huro gurna.")` 19×,
  `inflect(baugir, [PRS 3SG])` 13× in one run. `MaxToolRounds` caps rounds,
  not identical calls.
- **`-ctx none` works**: the model looks up what it needs (2–3 `lookup`s per
  sentence) and is as accurate as prefetched retrieval, at 1.5–2× the time
  and tokens. With retrieved context it almost never calls `lookup` (once
  in 7): it uses tools to *verify* (`check`, `inflect`), not to *fetch*.
  Sentence 3 needs no lookup in either mode, because it's grammar.md's own
  example and is already in the prompt.
- Tools fix exactly what retrieved alone got wrong: verb agreement
  (`naraxis` → `naraxe`), tense, word order.

### Fixed afterwards (the user said yes to all three)

- Killed the stale service on :8765.
- `llm/app.py`: messages are rendered before the Ollama call, so a template
  error is a 500 of its own. Only `ConnectionError` says "ollama not
  reachable" (503); any other failure is "ollama call failed: <type>" (502).
  Tests for both.
- **Repeated tool calls** (`Toolbox.runOnce`): a call identical to an
  earlier one for the same sentence (any attempt) isn't run again. It gets
  "repeated call: you already called X and got: … Use it, or call something
  different." The key is `ToolCall.String()`, which now prints `features`
  as a list however they're sent (`"PRS.3SG"` = `["PRS","3SG"]`). The CLI
  shows `↻ repeated call`.
- Rerun of `The dog eats the bread.` (retrieved + tools): **120 → 22 calls,
  423 → 151 s**, 6 repeats caught. Still ✗: the model gets `bauge` from
  `inflect` and writes `baugis`. Dedupe makes a stuck model cheaper, not
  right.

### Concepts

- **Agentic vs. prefetched RAG**: with prefetched context the retriever
  decides what the model sees; with `-ctx none` the model decides by calling
  `lookup`. Same accuracy here, but agentic costs more rounds, and every
  round resends the whole conversation.
- **One run per cell says little.** Sampling noise (sentence 5: a stall,
  then 27 s) can flip a cell, so 6/7 vs. 5/6 isn't a real difference. That's
  the case for Evals: more sentences, several runs, a pass rate with a spread.

### Start next session here

1. Evals (ROADMAP → Next up), then Coin new words.
2. Open idea for the "right form, wrong word" failure: after a draft fails
   `check`, put the `inflect` result for the failing slot into the feedback
   ("the verb must be bauge"), or try a larger local model.

---

## 2026-10-18 (later): Lexicon 323 → 629, word formation

### Done

- **306 new entries**: hate (verb *povisar*) + hatred, fortress, colours
  (pink, brown, grey, purple, orange; golden and silver derived from the
  nouns), and animals in new files: `mammals.yaml` (112),
  `birds.yaml` (76), `insects.yaml` (33), `fishes.yaml` (27),
  `creatures.yaml` (48). `colour` already existed (*tilho*).
- **Word formation** in `grammar.md` (draft): proto-level diminutive
  *-itto/-itta*, augmentative *-aron/-aran*, adjective *-io*, deverbal noun
  *-tia*, head-first compounds. 183 roots from `lnova gen`, 123 derived
  (lion *sersebo* → leopard *sersebito*, horse + river → hippopotamus
  *brecotrubo*). Each derived entry says how in `notes`; no new YAML field.
- Builder script (not in the repo): roots from `gen`, rejected if
  Levenshtein distance < 2 to any word, if `analyze` already reads them as a
  form, if they end in a diphthong (no suffix fits after one) or in `-h`.
- `validate` 0 problems, `evolve` 0 mismatches over 629, all tests pass.

### Key decisions (the user's choices)

| Area | Decision |
|---|---|
| Roots | **Mixed**: generator roots plus derivations from existing words |
| hate | Verb + noun |
| golden, silver | **Derived** from the nouns gold, silver |
| Homonyms | **Dotted glosses**: `fly.insect`, `bat.animal`, `crane.bird`, `silver.colour`... |

Mine: diminutives and augmentatives keep the base's gender, deverbal nouns
are feminine, a compound takes the head's gender (catfish *nauganujo* is f).
The lists hold the well-known animals only (≤150 per category, as asked),
not padded to 150.

### Open questions

- Reviewed by the user on the review page; committed as built.
- Generator weights count the 323 old roots; re-seed from roots only?
- `lnova llm -ctx full` now sends 629 entries: may not fit qwen's context.
- Distance-1 pairs left in: puppy *hurito* / jackal *hurlito* (from dog
  *huro* / wolf *hurlo*), and two more flagged on the review page.

---

## 2026-10-18: Tool use for LLM translation

### Done

- **Protocol** (`internal/llm/client.go`, `llm/app.py`): `Tool`,
  `ToolCall`, `ToolResult`, `Step` (one assistant message with calls + the
  results). `Request` gains `tools`, `steps` (current attempt), `final`;
  `Turn` keeps its steps; `Response.answer` is optional, plus `calls` and
  `content`. The service stays one model call per request: a tool round
  sends `tools` and no `format`, the final round `format` and no tools,
  with a "now give the final answer" user message. Output capped at
  512 tokens per tool round and 1024 per answer (`num_predict`).
- **Tools** (`internal/llm/tools.go`): `lookup` (same path as retrieval:
  tag, lemma, gloss, then search), `inflect` (`morph.Rules.Inflect`),
  `analyze` (`morph.Analyzer`), `check` (`Checker.Verify` on a draft, with
  the sentence's `Expectation`). Results are plain text; a bad call returns
  `error: …` and the loop goes on. Defined in Go next to the code that runs
  them; the service passes the schemas through.
- **Loop** (`internal/llm/loop.go`): per attempt, tool rounds until the
  model stops calling or `MaxToolRounds` (8), then `final`. Tokens and time
  add up over the attempt's calls. A retry carries the failed attempt's
  steps and the feedback, and can use tools again.
- **Context** `none`: grammar, morphology and examples, no dictionary.
- **CLI**: `lnova llm -tools [-tool-rounds N] [-ctx retrieved|full|none]`;
  each call is printed as `→ inflect(eat, [PRS 3SG]) = bauge (eat-TH.3SG)`.
- A client timeout now says "timed out … check ollama ps" instead of the
  "start the service" hint.

### Key decisions (the user's choices)

| Area | Decision |
|---|---|
| Loop | **Go drives it**; the Python service stays stateless |
| Tools | lookup, inflect, analyze **and check** |
| Context | `-tools` is a flag for any `-ctx`, plus `-ctx none` |

Mine, from the first real run: `inflect` also accepts the dictionary
word. The model kept sending `inflect(baugir, …)` instead of
`inflect(eat, …)`, got "no entry", looked the word up, and sent the same
call again until the round cap. A tool should accept what the model
naturally sends.

### Measurements (same 7 sentences; one run each)

| sentence | retrieved | retrieved + tools | tool calls |
|---|---|---|---|
| The woman sees the river. | ✓ | ✓ | 1 (check) |
| The men will drink the water. | ✓ | ✓ | 4 |
| Didst thou see the woman's dog? | ✓ | ✓ (2nd; mislabeled id) | 2 |
| The dog eats the bread. | ✗ `baugir` | ✗ `bauge` but `gurnah` (PL) | 15 |
| She didn't see the big house. | ✗ | **✓** (2nd, via `inflect`) | 6 |
| Who saw the men? | ✗ `naraxis` | **✓** (via `check`) | 3 |
| We will love the god. | ✗ | not measured (Ollama stuck) | |
| **passed and right** | **3/7** | **5/6** | |
| time per sentence | 5–20 s | 19–145 s | |

`-ctx none -tools` isn't measured: `ollama serve` got stuck mid-run
(`ollama ps` shows the model "Stopping..." with no runner). Every request
waited for the 10-minute client timeout. Restart Ollama and rerun.

What the transcripts show:

- **Tools help where one call fixes one slot.** `Who saw the men?`:
  `check` said "needs see-TH-PST-3SG", the model tried twice and got `OK`.
  The big house: `inflect(narar, [PST 3SG]) = naraxe` on the retry.
- **Getting the right form isn't using it.** On the first real run,
  `inflect` returned `bauge` and every later draft still had `baugir`.
  In the measured run, `bauge` only made it into the answer on attempt 3,
  which was then wrong in another slot (`gurnah`, after the model called
  `inflect(gurna, [PL])` for a singular noun on its own).
- **Repeated calls**: the model sends the same failing call
  (`inflect(huro, [PRS 3SG])` on a noun) round after round. That's the
  classic agent "doom loop"; the round cap is what ends it.
- **Cost**: every round resends the whole conversation, so a tools
  attempt is 17K–42K prompt tokens instead of ~5K, and 2–7× slower.

### Concepts

- **Function calling**: the model emits a structured call
  (`<tool_call>{"name":…,"arguments":…}` in qwen's template, parsed by
  Ollama into `message.tool_calls`); it never runs anything. The host
  runs it and answers with a `role: tool` message.
- **Agent loop (ReAct)**: act → observe → repeat → answer. Here it runs
  inside each attempt of the generate-and-test retry loop.
- **Tools vs. constrained decoding**: a JSON-schema grammar masks every
  token of a tool call, so tools and `format` go in separate calls.
- **Tool design for small models**: accept loose input (word or id,
  `"PST.3SG"` or `["PST","3SG"]`), return short text, put the fix in the
  error message ("use lookup to find the id").
- **Agentic vs. prefetched RAG**: `-ctx none -tools` makes the model fetch
  its own context; still to be measured.

### Start next session here

1. Restart `ollama serve`, then finish the measurement (ROADMAP → Next up 1).
2. Ideas if tools stay weak, for the user to decide: a newer or larger
   local model with better tool use; return the `check` verdict
   automatically after each `inflect`; drop repeated identical calls.
3. Then Coin new words, or Evals first.

---

## 2026-10-17: Meaning check for LLM answers

### Done

- **Glosser rule** (`internal/gloss/gloss.go`): a pronoun without case
  forms (`ifa` who, `itu` what, `zunhu` nothing) as the subject agrees 3SG,
  **only when the verb has an object**. Without one, `Ifa naraxis?` is
  "whom did you see?" (fronted object + pro-drop) and stays valid.
- **Frames** (`internal/gloss/frame.go`): `Result.Clauses()` cuts each
  clause into Wh / Subject / Neg / Verb / Object, each word as entry id +
  features. Question-word ids come from `english/words.yaml` (WhP, WhAdv)
  via `translate.New` → `Glosser.Wh`.
- **Meaning check** (`internal/llm/meaning.go`): `Checker.Expect(english)`
  runs once per sentence. If the English parses, every rule-based
  translation is a reference, and the answer's clauses must equal one of
  them (a dropped subject pronoun is fine). Otherwise a **lexical**
  fallback: each English content word needs one of its entries, and the
  answer adds no content words. Meaning problems fail the answer and go
  back as feedback, naming glosses (`see-TH-PST-3SG`), not spelled words.
  Ties between references go to the one sharing the most entries with the
  answer. `lnova llm` prints `meaning check: frame, N reference(s)`.
- `candidates()` factored out of `Builder.Retrieved`: retrieval and the
  lexical check share it.

### Key decisions (the user's choices)

| Area | Decision |
|---|---|
| Meaning | Frame comparison against the rule-based translations, lexical fallback when the English doesn't parse |
| Form rules | Only the question-word 3SG rule; the other gaps (no finite verb, `na` after the verb, possessive subject, adjective order) are caught by the frame check when the English parses |
| Severity | Meaning mismatches fail the answer and are fed back |

### Measurements (same 7 sentences as last session)

| | retrieved | full |
|---|---|---|
| passed, before | 7/7 | 6/6 |
| passed, now | 3/7 | 3/7 |
| passed and right, now | 3/7 | 3/7 |

No false passes now. The 3 that pass are still grammar.md's examples
(in the prompt). On the 4 new sentences the model fixes the slots it's
told about (articles, `hane` → `hana`, adjective after the noun) but
**can't inflect the verb** even when given the exact gloss: it produces
`baugiru` for eat-TH.3SG, `eluvirxu`/`eluvareh` for love-FUT-1PL. A
7B model doesn't learn a paradigm from a table in the prompt. That's
the case for the next step, tool use: let it call `inflect`.

### Concepts

- **Predicate–argument structure**: who did what to whom, when, negated
  or not. Two sentences mean the same if those slots match, whatever the
  spelling. Reading both the answer and the reference through the *same*
  glosser keeps the comparison symmetric.
- **Reference-based evaluation with many references**: "you" × "love"
  gives up to 20 correct translations; an answer must match one, and the
  feedback compares it with the closest one (as BLEU uses multiple
  references, but with exact slots instead of n-grams).
- **Precise feedback isn't enough**: the model knew what it needed
  (eat-TH.3SG) and still couldn't produce the form. Knowing vs. doing:
  the gap tools are meant to close.

### Start next session here

1. `ROADMAP.md` → "▶ Next up": Tool use.
2. Decided: `ifa` stays invariable (no who/whom/whose forms, like
   Portuguese *quem*); as subject it takes 3SG; without an object the
   ambiguity is left to context. Written into grammar.md → Questions.
3. The lexical fallback only runs on English the Phase 4 grammar rejects;
   it's tested but hasn't been seen on a real model answer yet.

---

## 2026-10-17: Phase 5 LLM translation (local model)

### Done

- **`llm/`**: a Python FastAPI service (managed with `uv`) in front of
  Ollama. One request = one attempt. It renders `prompts/system.j2`
  (rules, grammar.md, morphology.yaml, grammar.md's examples, then the
  dictionary as `id | word | pos | gender | gloss | definition` lines),
  calls `ollama.chat` with the pydantic `Answer` schema as `format`, at
  temperature 0, and validates the reply. `GET /health`; pytest covers
  prompts, message order and schema with a fake Ollama client.
- **`internal/llm`**: `client.go` (stdlib HTTP, start hint when the
  service is down), `context.go` (`Full` / `Retrieved`), `check.go`
  (`Checker.Verify`), `loop.go` (`Translator.Translate`: generate →
  check → resend the whole conversation with feedback, max 3). The context
  window `num_ctx` is sized from the prompt (≈3 bytes/token + room).
- **`lnova llm [-ctx retrieved|full] [-model] [-attempts] [-url] [-compare]`**
  shows every attempt with its problems (`!`), remarks (`~`) and the
  model's notes, then the gloss and the rule-based verdict.
- **Glosser bug fixed**: a word with *no* reading (`blomp`) got the notes
  "is a second verb in the clause" and "is SBJ after the verb", because
  `remove` treated "no readings at all" like "would remove the last one".
  Regression test added.

### Key decisions (the user's choices)

| Area | Decision | Why |
|---|---|---|
| Model | **Local only**: Ollama + `qwen2.5:7b-instruct-q4_K_M` | No external API; RTX 3060 6 GB |
| Python | **Wrapper service** in `llm/` | Python's LLM ecosystem for later steps (embeddings, tools) |
| Context | **Switchable**: `full` / `retrieved` | Compare full context with RAG-style retrieval |
| Output | **Structured JSON** (translation + per-word id/features + notes) | Lets the checker compare claims with the analyzer |
| Retry | **Glosser feedback, max 3**, every attempt shown | See the model correct itself (or not) |

Smaller choices (easy to change): the Go side owns the loop and
correctness, Python is stateless; retrieved = closed classes
(function.yaml, pronouns.yaml) + tagger/lemmatizer entries + dictionary
search for unknown words (top 3, word match or better); a claimed id that
isn't in the lexicon or wrong features are *remarks*, a claimed id that's
a different real entry is a *problem*.

### First measurements (informal, 7 sentences)

| | retrieved (≈5K tokens) | full (≈9.8K tokens) |
|---|---|---|
| passed the checker | 7/7 (2 after a retry) | 6/6 (1 after a retry) |
| actually right | 3/7 | 2/6 |
| first attempt | 5–20 s | ~30 s, 18% of the model on CPU |
| retry | 4–9 s | 9 s |

The only right ones are grammar.md's three examples, **which are in the
prompt** (few-shot). New sentences all passed and were all wrong:

- `The dog eats the bread.` → `Huro baugir lu gurna.`: infinitive as the
  verb, no article. The glosser only checks a clause that *has* a finite verb.
- `She didn't see the big house.` → `Hane na nara gumo hoida.`:
  possessive `hane` as subject, present not past, adjective before the
  noun, no article. (Full mode: `nara na`, NEG after the verb, `lirma` = white.)
- `Who saw the men?` → `ifa naraxis dubroh`: 2SG verb with *who*.
- `We will love the god.` → first `Muh noidareh` (agreement error,
  caught). The model then fixed it by **changing the subject** to `Selah`
  (polite you) rather than the verb: it satisfied the check but changed
  the meaning.

### Concepts

- **Constrained decoding**: Ollama turns the JSON schema into a grammar
  (llama.cpp GBNF) and masks every token that would break it while
  sampling. The reply always parses, but the *content* isn't checked.
  Same idea as the Earley parser, used to generate instead of recognize.
- **RAG vs. full context**: retrieval cut 323 entries to ~26 and the
  prompt by half. The grammar (~4.5K tokens) is now most of it.
  Retrieval can only miss what the tagger misses.
- **KV cache and prefix reuse**: retries are 3–5× faster than first
  attempts because the prompt prefix is already in the KV cache. Changing
  `num_ctx` reloads the model, which is why alternating modes was slow.
  The 9.8K `full` prompt needed a 14K window (784 MiB of KV cache) and pushed
  18% of the layers to the CPU.
- **Few-shot contamination**: a test sentence that appears in the prompt
  measures copying, not translating. Evals need held-out sentences.
- **Goodhart's law / validator gaming**: feedback that only names the
  broken rule invites the cheapest fix that silences it (`Muh` → `Selah`).
  A form checker is necessary but not sufficient; meaning needs its own
  check (claimed ids vs. the English, or the rule-based translation).
- **Self-correction loops**: the model sees its answer and the analyzer's
  reading (gloss lines). That fixed made-up words (`xumo` → `gumo`) and
  agreement in one step.

### Start next session here

1. `ROADMAP.md` → "▶ Next up": decide which validator gaps to close
   (glosser rules for form; a claimed-ids-vs-English check for meaning).
2. Run: `ollama serve`, `cd llm && uv run uvicorn app:app --port 8765`,
   then `lnova llm -compare "…"` (`-ctx full` for the whole dictionary).
3. Everything from this session is committed (`Phase 5: LLM translation
   with a local model`).
4. Before measuring anything again: use sentences that are **not** in
   grammar.md (those are in the prompt). They'll seed the Phase 5 evals step.

Notes: `uv` picked Python 3.12 from the venv (fine; needs ≥3.12). The
answer schema lives in `llm/app.py` (`Answer`) and is mirrored in
`internal/llm/client.go`: change both together. A nil Go slice is sent as
JSON `null`, which pydantic rejects, so optional lists use `omitempty`.

---

## 2026-10-17: Phase 4 English → Rifatian

### Done

- **`internal/earley`**: grammar file parser (`A -> B C | D`, `#`
  comments, line numbers in errors) and an Earley parser that returns
  **every** tree (capped), with tokens that can carry several categories.
  Left recursion works; empty rules aren't allowed.
- **`internal/english`**: the prose Treebank tokenizer (`didn't` → `did
  n't`, `woman's` → `woman 's`) and averaged-perceptron tagger, plus
  **`language/english/words.yaml`**, which overrides the tagger for function
  words and archaic forms (`thou`, `didst`, `her` = Pron or Poss, `you` =
  4 pronouns) and lists irregular pasts and plurals. Open-class words are
  lemmatized by suffix stripping (`-s -es -ies -ed`, doubled consonants,
  archaic `-est -eth`) and looked up by the lexicon's gloss head
  (`love` → the five `love.*` verbs). Each word also gets a reading for
  every category its lemma has in the lexicon, so a tagger mistake can't
  block the parse.
- **`language/english/grammar.cfg`**: statements, `Aux`/`Neg` predicates,
  yes/no questions, subject and object wh-questions, wh-adverbs, `'s` and
  possessive determiners, adjectives, bare plurals.
- **`internal/translate`**: tree → clause (wh, subject, aux, neg, verb,
  object) → Rifatian: auxiliary dropped and its tense moved to the verb,
  `na` before the verb (also for a `nothing` object: double negation),
  question word first, adjectives after the noun, `-s` on the possessor's
  noun, pronoun case by position, verb agreement (sela → 3SG via
  `agrees`). Every combination of trees × word choices is generated,
  deduplicated, ranked, and **glossed back** to check it.
- **`lnova parse`** (tokens with PTB tags and categories, then every tree
  with its tagger score), **`lnova translate [-n] [-tree]`**, the
  **`/translate`** page (live htmx, words linked to entries, gloss of the
  best translation; hot-reload watches `english/`), and **`lnova
  validate`** checks words.yaml ids and categories against the lexicon and
  the grammar.
- `gloss.Examples` (moved from the glosser's test): **grammar.md's
  examples are golden tests for the translator too**. Every one comes out
  exactly.

### Key decisions (the user's choices)

| Area | Decision | Why |
|---|---|---|
| Tagger | **prose** (statistical, Penn Treebank) | A first contact with statistical NLP; the word table covers its blind spots |
| Parser | **Earley chart parser + CFG file** | The core parsing algorithm; the grammar is data, ambiguity gives several trees |
| Scope | SVO, negation, questions, archaic thou | Covers every example in grammar.md |
| Ambiguity | **All translations, ranked** | Nothing hidden; shows how fast ambiguity multiplies (`You love them.` = 40) |

Smaller choices made while coding (easy to change): ranking is (tagger
agreement of the tree, sum of each word's choice index, tree order); the
web page shows 20; an exact gloss ranks before a dotted sense; "you" ranks
informal singular first; unknown English words become `?word` (exit 1).
prose v1.2.1 is archived upstream but stable; its model is embedded.

### Concepts

- **POS tagging as classification**: the averaged perceptron scores each
  tag from features (the word, its suffix, the previous two tags). It was
  trained on news text, so it tags `Didst` as a proper noun and `thou` as a
  noun. And one unknown word (`dragon`) can derail its neighbours (`eats`
  → plural noun, `bread` → verb). Hence the hybrid design: a statistical
  guess, a table for the closed classes, and the grammar as the judge.
- **Earley's algorithm**: predict / scan / complete over a chart of
  dotted items; O(n³) at worst, works on any CFG without converting to
  Chomsky Normal Form (which CYK needs). Trees are read back from the
  completed items. The PP-attachment test (`saw the man with the
  telescope`) shows why parsers return forests. Two PPs give 5 trees: the
  Catalan numbers.
- **Transfer-based MT**: analysis → transfer → generation, the
  pre-statistical architecture (SYSTRAN, 1960s on). The transfer rules
  are exactly the English/Rifatian differences in grammar.md's overview.
- **Gaps**: in `What did you see?` the object is fronted, leaving a gap
  after `see`. English needs a special rule for it (`Gap`); Rifatian
  doesn't move anything except the question word.
- **Generate-and-test** again: every output is glossed back by the
  Constraint Grammar glosser, the same trick as the analyzer.

### Language decisions (the user's, recorded in grammar.md)

| Question | Decision |
|---|---|
| Possessor with an adjective | **-s on the noun**: `lu mena-s lirma huro` |
| Several adjectives | **Any order** after the noun |
| Negative word before the verb | **No na**, as in Portuguese *Nada caiu*: `Zunhu zuvixe.` |

The translator already did all three; the "undecided" note is gone and
each has a test.

Out of scope for now: copula (`is big`), prepositional phrases, -ing
forms, modals other than will, more than one sentence per input.

### Start next session here

1. `ROADMAP.md` → "▶ Next up": Phase 5, LLM translation.

---

## 2026-10-17: Phase 4 interlinear glossing

### Done

- **`internal/gloss`**: `New(analyzer).Gloss(sentence) → *Result`. The
  tokenizer keeps punctuation (`hono?`, `— Naraxu.`) and splits clauses at
  `. , ? ! ; : —`. Each word gets every reading from `morph.Analyzer`, then
  rules run per clause:
  1. the **verb** is the first word with a finite reading (known roots
     before guesses); finite readings of other words are removed;
  2. **case by position**: SBJ pronoun readings after the verb and OBJ
     readings before it are removed;
  3. **agreement**: the subject is the nearest word before the verb
     (skipping `na` and adjectives; none = pro-drop), and readings of
     subject or verb that don't agree are removed.
  A rule never removes a word's last reading. If it would, the sentence
  gets a **note** instead ("hono is SBJ after the verb", "mu (1SG) doesn't
  agree with nara (3SG)").
- **`morph`**: `Analysis.Finite()` (verb person) and
  `Analyzer.Agreement()` (the verb person a subject takes).
  **`morphology.yaml`**: `you_polite` gets `agrees: "3"` (approved with
  the plan); grammar.md already said sela takes 3sg verbs.
- **`lnova gloss <sentence>`**: two aligned lines, then remarks (removed
  readings, guesses, notes). Exit 1 if a word has no analysis.
- **`/gloss`** page: live as you type (htmx, like search). Words link to
  their entries, guesses are muted, unknown words are red, and remarks
  are listed below. There's a "Gloss" link in the header.
- Tests: **every interlinear block in grammar.md** is a golden test, read
  from the file; ambiguity, agreement, pro-drop, clauses, guesses,
  tokenizer, column layout, CLI, pages.

### Key decisions (the user's choices)

| Area | Decision | Why |
|---|---|---|
| Ambiguity | **Word order + agreement**, rejected readings listed with the rule | A first taste of syntax; the reasons make the choice visible |
| Surfaces | **CLI + web page** | Phase 4's done-when: the app glosses any sentence |
| Unknown roots | **Best guess, marked** (`?-TH-PST-1SG`) | Same as the analyzer; useful for LLM output |

Smaller choices made while coding (easy to change): rule violations become
notes rather than errors; an unresolved ambiguity takes the analyzer's
first reading and lists the others as "also".

### Concepts

- **Interlinear glossing** (Leipzig Glossing Rules): line 1 is the word
  split at morpheme boundaries, line 2 has one label per morpheme, aligned
  word by word. Hyphens match between the two lines.
- **Constraint Grammar** (Karlsson 1990): no tree, just readings and rules
  that SELECT or REMOVE readings in context. "Never remove the last
  reading" keeps it robust: a bad sentence still gets a gloss, plus a
  note. Real CG grammars (VISL, Giellatekno for Sámi) have thousands of
  such rules; ours has three.
- **Agreement as evidence**: the verb ending `-a` (3SG) tells you which
  reading of the subject is meant. Morphology on one word disambiguates
  another word. That's syntax.
- **Pro-drop**: no subject before the verb is fine, since the verb ending
  carries the person. So "no subject" isn't an error, only a mismatch is.
- **Polite pronouns and agreement**: `sela` is 2nd person in meaning but
  3rd in grammar (like *você*, *Usted*). The gloss says `2SG.POL` and
  agreement uses 3SG, so meaning and form are separate features.
- **Golden tests from documentation**: the examples in grammar.md are now
  checked by code, so the doc and the engine can't drift apart.

### Found on the way

- grammar.md used `hono` (subject) as an object twice. Fixed to `honi`
  (your call), as the pronoun table says.

### Start next session here

1. `ROADMAP.md` → "▶ Next up": English → Rifatian.

---

## 2026-10-17: Phase 4 analyzer

### Done

- **`internal/morph/analyze.go`**: `NewAnalyzer(rules, lex)`,
  `Analyze(word) → []Analysis` (entry or guess, lemma, POS, class, `Form`).
  Steps:
  1. bare words: adjectives, particles…, and verb infinitives (`nar-ar`,
     `see-INF`);
  2. pronouns: an inverted table, form → (id, number, case);
  3. nouns: peel POSS then PL (reverse slot order), each optional, both
     shapes;
  4. verbs: peel person ending, tense marker, theme vowel (→ class); a
     person with no theme vowel (PRS 1SG) tries every class;
  5. verify each candidate by inflecting it again and comparing the
     spelling (and that no morpheme break cuts a digraph);
  6. only if nothing known fits: the same candidates as guesses, against a
     made-up entry, if the lemma obeys the phonotactics and the stem has a
     vowel.
- **`lnova analyze <word>...`**: one line per analysis, `(guess)` marked,
  `—` and exit 1 for a word with none.
- **Search**: a one-word query also shows "naraxu is a form of narar
  (verb, “see”): PST.1SG nar-a-x-u" (known roots only), in the full page
  and the htmx fragment.
- Tests: grammar.md forms, ambiguity, guesses, errors, CLI, pages, and a
  **round trip**: every cell of every paradigm (2,177 forms) analyzes back
  to its entry and features, with no guesses next to a known root.

### Key decisions (the user's choices)

| Area | Decision | Why |
|---|---|---|
| Algorithm | **Affix stripping + verification** | Real reverse parsing; the generator becomes the test oracle |
| Unknown roots | **Reported as guesses** (`?pumar`), below known roots | Useful for checking LLM output and coining words (Phase 5) |
| Ambiguity | **All analyses, ranked** | Choosing one needs context: that's syntax, the glosser's job |
| Surfaces | CLI, search, bare words too | The glosser can make one call per word, whatever its part of speech |

Two smaller choices made while coding (easy to change):
- Guesses only appear when no known root fits, and the ones that explain
  more of the word (more morphemes) rank first.
- Infinitives are glossed `INF` (Leipzig standard). It's a display label,
  not a change to the grammar.

### Concepts

- **Analysis is harder than generation.** Generation is a function (one
  input, one output). Analysis is a relation: one word can have many
  readings, and most stripping paths lead nowhere. Stripping guesses;
  something has to check the guess.
- **Analysis by synthesis**: instead of writing the reverse rules twice,
  strip loosely (any suffix that matches as a string) and let the generator
  confirm. Allomorph conditions (`-s` only after a vowel) and theme/class
  pairings are then checked for free, by the code that defines them.
- **Nondeterminism / search**: `menahes` peels three ways (`menahes`,
  `menah+es`, `mena+h+es`); only the last is a real noun. The noun stripper
  is a small recursive search over optional slots, the same shape as
  backtracking parsers.
- **Suppletion can't be stripped**: `mu/mi/me` share no stem, so pronouns
  are a lookup table. Real analyzers mix rules and lists the same way.
- **Morphological vs. syntactic ambiguity**: `sela` is SBJ or OBJ by its
  form alone; the sentence decides. The analyzer's job is to list
  readings, not to choose.
- **Overgeneration**: unknown `pumaxu` gets 5 guesses (PST.1SG of
  *pumar*, PRS.1SG of *pumaxar/er/ir*, noun *pumaxu*). Ranking by how much
  of the word is explained is a simple version of what statistical
  analyzers do with probabilities.
- Real-world names: this is a small **two-level / finite-state morphology**
  problem solved the simple way; tools like HFST or foma compile the same
  tables into a transducer that runs both directions.

### Bug caught on the way

- Template `.Gloss` picked the form's gloss (`woman-PL-POSS`) over the
  entry's (`woman`): Go resolves an embedded method at depth 1 before an
  embedded field at depth 2. `form.Form` is now a named field.

### Start next session here

1. `ROADMAP.md` → "▶ Next up": interlinear glossing.

---

## 2026-10-17: Phase 3 closed, Phase 4 morphology generator

### Done

- **Phase 3 closed** (your call: the app beats grepping YAML).
- **`language/morphology.yaml`**: the grammar.md inflection tables as data.
  Verbs: persons, 3 classes (infinitive ending → theme vowel), tenses with
  a marker (`x`, `r`), person endings, `no_theme` (PRS 1SG) and theme
  overrides (class III `e` in PRS 2SG/3SG/3PL). Nouns: PL and POSS as
  two-shape suffixes in slot order. Pronouns: whole forms per lexicon id.
- **`internal/morph`**: `Inflect(entry, feats...)` → `Form` (morphemes
  with glosses: `nar-a-x-u` / `see-TH-PST-1SG`), `Paradigm(entry)` → table,
  `Check(lex, rules)` → problems.
- **`lnova inflect <id> [features]`**, and `lnova validate` now also checks
  every inflected form: 323 entries, 0 problems.
- **Entry page**: Conjugation / Declension / Forms table, segmentation
  under each form, gloss as tooltip. `morphology.yaml` hot-reloads like the
  other files.
- Tests: every form in grammar.md, lexicon-wide `Check`, parse errors,
  CLI, pages, reload of a changed/broken `morphology.yaml`.

### Key decisions (the user's choices)

| Area | Decision | Why |
|---|---|---|
| Rules source | **Data file** `morphology.yaml` | Draft endings change without code; the analyzer reads the same tables |
| Scope | **Verbs + nouns + pronouns** | Everything the example sentences need for glossing |
| `it` plural | **None**; plural things are `honoh` | Like Portuguese *eles*; noted in grammar.md |

### Concepts

- **Morpheme, slot, paradigm**: a verb is stem + theme + tense + person,
  always in that order; the paradigm is the grid of all feature cells.
- **Allomorphy**: `-h`/`-eh`, `-s`/`-es` are one morpheme each, shape
  chosen by the preceding sound (*phonologically conditioned*). `menahes`
  gets `-es` because plural `-h` is a consonant, no special rule needed.
- **Fusion**: `nar-a` = `see-TH.3SG`, one vowel carrying two meanings.
- **Suppletion**: pronouns are listed, not derived.
- **Item-and-arrangement** (the engine glues morphemes) vs.
  **word-and-paradigm** (the table you read).
- **Boundaries vs. spelling**: gluing strings can create a digraph across
  a morpheme boundary (`pul` + `hu` would read `p·u·lh·u`). `Check`
  compares the word's segments with its morphemes' segments to catch it.
  No current ending triggers it.

### Known limitations

- Still not checked in a real browser (Chrome extension not connected);
  checked with tests and curl.
- Inflection works on the modern form only; proto morphology isn't modelled.

### Start next session here

1. `ROADMAP.md` → "▶ Next up": the analyzer.

---

## 2026-10-17: Phase 3 htmx live search

### Done

- **Search as you type.** The input has `hx-get="/"`,
  `hx-trigger="input changed delay:200ms, search"`, `hx-target="#results"`,
  `hx-push-url="true"`. The form and button stay, so it works without JS.
- **One handler, two shapes.** `handleSearch` checks the `HX-Request`
  header: htmx gets only the `results` template (defined in
  `search.html`), a browser gets the full page. `Vary: HX-Request` keeps
  caches from mixing them. `render` now calls a shared `execute`.
- **htmx 2.0.4 vendored** in `cmd/server/static/`, embedded with
  `//go:embed`, served at `/static/`.
- Tests: `TestPages` gained an `htmx` column (fragment has results but no
  `<!doctype>`/header/input; escaping; static file).

### Key decisions (the user's choices)

| Area | Decision | Why |
|---|---|---|
| htmx source | **Embedded in the binary** | Works offline, version pinned in the repo, same as the templates |

### Concepts

- **Hypermedia / HTML over the wire**: the server sends ready HTML
  fragments and htmx swaps them in. All rendering stays in Go templates;
  no JSON API, no client-side templates.
- **Debouncing**: `delay:200ms` waits until typing pauses, so `exa`
  sends one request, not three. `changed` skips keys that don't change
  the value (arrows).
- **URL as state**: `hx-push-url` updates `/?q=…`, so refresh, back and
  sharing a link give the same page (the full-page path renders it).

### Known limitation

- The reload banners only refresh on a full page load, not on live-search
  swaps. Fixable later with `hx-swap-oob` if it bothers you.
- Not checked in a real browser this session (Chrome extension wasn't
  connected); checked with tests and curl.

### Start next session here

1. `ROADMAP.md` → "▶ Next up": decide whether Phase 3 is done.

---

## 2026-10-17: Phase 3 hot reload

### Done

- **`cmd/server` reloads `language/`** when a file changes. A goroutine
  stats `phonology.yaml`, `sound-changes.txt` and `lexicon/*.yaml` every
  500ms (`-poll`, 0 = off) and compares size + mtime; a new or deleted
  lexicon file counts as a change too.
- **Two error levels**, shown as a banner on every page:
  - the files don't load (bad YAML, a rule that won't parse): the old data
    stays, red banner with the error;
  - they load but `lexicon.Validate` finds problems: the new data is used,
    yellow banner with the first 10 problems in `lnova validate` format.
- `load(langDir)` split out of `newHandler`; `base.html` wraps every page
  (`{{template "content" .Data}}`), so the page templates didn't change.
- Tests: `TestReload` (add, break, fix, validate problem, on a temp copy
  of `language/`), `TestStamps`. Passes under `go test -race`.

### Key decisions (the user's choices)

| Area | Decision | Why |
|---|---|---|
| Watching | **Poll mtimes**, stdlib | ~17 files, a few stat calls per tick; works the same whether an editor writes in place or renames a temp file over the original (fsnotify needs special handling for that) |
| Errors | **Two levels**: load error keeps old data, validate problems don't block | One typo in a new word shouldn't hide every other edit |
| Concurrency | Immutable snapshot + `atomic.Pointer` swap | A request reads the pointer once and sees one consistent state; readers never wait on a lock |

### Concepts

- **Polling vs. events**: polling asks "did anything change?" on a timer;
  events (inotify, behind fsnotify) have the OS push a notice. Events are
  instant but tied to how files are written; polling costs a little lag.
- **Read-copy-update**: never change shared data in place. Build a new
  copy and swap one pointer. Old readers finish with the old copy and the
  GC frees it.

### Start next session here

1. `ROADMAP.md` → "▶ Next up": htmx live search.

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
