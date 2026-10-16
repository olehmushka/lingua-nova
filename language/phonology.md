# Rifatian phonology

## Design brief

- **Feel**: Western Romance, between Brazilian Portuguese and Occitan, plus one
  Sumerian trait: the sound /x/ (Ukrainian "х").
- **Rhythm**: Brazilian Portuguese: open syllables, every vowel pronounced
  clearly, no vowel reduction.
- **No nasal vowels** (no Portuguese -ão / -em endings).
- **Easy for a Ukrainian speaker**: every sound below exists in Ukrainian or
  is very close to one.
- **Spelling**: English letters only, no diacritics, Portuguese/Occitan-style
  digraphs. Each spelling has exactly one pronunciation.

## Two layers

The vocabulary is invented, so we invent the **proto-language** as well.
The modern language comes from it through [sound changes](sound-changes.txt).
Both layers use the same spelling; proto forms are written with a `*`
(`*folia` → `folha`).

| Layer | Consonants |
|---|---|
| Proto | p b t d c g · f v s z h · m n · l r |
| Modern | proto + **lh nh x j ch**, the palatals |

The palatals don't exist in the proto-language. They come from consonants
followed by *i* + vowel, just as in real Romance history (Latin *filia* →
Portuguese *filha*). That gives the modern language natural-looking
alternations for free.

## Consonants (modern)

| Spelling | IPA | Ukrainian ≈ | Notes |
|---|---|---|---|
| p | /p/ | п | |
| b | /b/ | б | |
| t | /t/ | т | |
| d | /d/ | д | |
| c | /k/ | к | **always** /k/, also before e, i (`ce` = /ke/) |
| g | /g/ | ґ | **always** /g/, also before e, i (`gi` = /gi/) |
| f | /f/ | ф | |
| v | /v/ | в | a real /v/, not [w] |
| s | /s/ | с | **always** /s/, also between vowels (unlike Portuguese) |
| z | /z/ | з | |
| x | /ʃ/ | ш | as in Portuguese *xícara* |
| j | /ʒ/ | ж | as in Portuguese *já* |
| ch | /tʃ/ | ч | |
| h | /x/ | х | **the Sumerian trait**; never silent |
| m | /m/ | м | |
| n | /n/ | н | always a plain [n], never nasalizes the vowel |
| nh | /ɲ/ | нь | as in Portuguese/Occitan |
| l | /l/ | л | |
| lh | /ʎ/ | ль | as in Portuguese/Occitan |
| r | /ɾ/ | р | a single tap; no rr |

Letters **k, q, w, y are not used**. Leaving them out keeps the Romance look.

## Vowels

| Spelling | IPA | Ukrainian ≈ |
|---|---|---|
| a | /a/ | а |
| e | /e/ | е |
| i | /i/ | і |
| o | /o/ | о |
| u | /u/ | у |

All vowels are pronounced fully in every position (Brazilian clarity: no
reduction, no nasalization).

**Diphthongs** (one syllable): `ai ei oi au eu ou`.
Every other vowel pair is two syllables: `dia` = di-a.

## Phonotactics

Syllable shape: **(C)(C)V(C)**, with open syllables (CV) by far the most common.

- **Onset clusters**: stop or f + r/l: `pr br tr dr cr gr fr pl bl cl gl fl`
- **Codas** (syllable-final consonant): only `r l s h`
- **Word-final**: a vowel, or `r l s h`. Never `n` or `m` (no nasal endings).
  Final `h` is allowed in any word, not only in the plural/3pl endings. So a
  final `-h` is not always a plural, just as English final *-s* isn't (*bus*).
  The analyzer has to check the lexicon.
- No double consonants in modern words.
- **No three vowels in a row.** A vowel pair is either a diphthong (`hoi.da`)
  or two syllables (`di.a`), never part of a longer run. The validator
  enforces it.
- **`l`, `n`, `c` are never followed by `h`**, in proto or modern words.
  `lh nh ch` are digraphs, so l+/x/ would be spelled the same as /ʎ/
  (`bilha`: bi-lha or bil-ha?). Banning the sequence keeps every spelling
  readable in one way only. The validator enforces it.

### Proto phonotactics

The proto-language has the same onsets, diphthongs and banned sequences,
but different codas and finals:

| | Proto | Modern |
|---|---|---|
| Codas (inside a word) | `r l s`, plus the first half of a geminate | `r l s h` |
| Word-final | a vowel, or `r n m` | a vowel, or `r l s h` |
| Double consonants | `pp tt cc`, only between vowels | none |
| Hiatus (`V.V`) | only after `i` (`*u.si.a`) | after any vowel (`di.a`) |

- **Geminates** (double consonants) are only `pp tt cc`, because sound change
  rule 3 removes exactly those. A proto `*ss` would survive into the modern
  language, where it's illegal. They stand only between vowels
  (`*cup.pe`, `*it.tun`), never next to another consonant or at a word edge.
  Their job is to give the modern language voiceless `p t c` between vowels:
  `*patta` → `pata`, while `*pata` → `pada`.
- **Final `n m`** exist only in proto. Rule 4 deletes them, so `*cutan` →
  `cuda` hides a nasal in its history.
- **Hiatus only after `i`.** A syllable may start with a vowel right after
  another vowel only when that vowel is `i`: the `-ia`/`-io` endings
  (`*u.si.a`, `*bu.zi.o`). Rule 1 turns most of them into a palatal
  (`*esia` → `exa`). `*a.ar`, `*nei.va.o` or `*au.mu.i` are not proto roots.
- **No `h` coda in proto roots.** A modern coda `h` comes from a suffix
  (the `-h` plural), not from a root.

### Word generator

`lnova gen` invents new proto roots that look like the existing ones.
`phonology.yaml` → `proto: generator:` holds its templates: a weighted
syllable count (mostly two syllables), then weighted syllable shapes for
each position. Shapes depend on position: first syllables vary (`CV`, `CVV`,
`CVC`, `V`, `CCV`...), middle ones are nearly always `CV`, and last ones are
`CV`, `CVC` or a bare `V` (the `-ia`/`-io` hiatus endings, `*u.si.a`). The
starting weights are the counts in the 104 roots; edit them to steer the
generator.

Which sound fills each slot is weighted by how often it fills that slot in
the lexicon, plus one, so sounds the roots never use in a slot are rare but
possible. A coda inside a word may be the first half of a geminate; the
next syllable then starts with the same consonant (`*cup.pe`). Every
candidate is checked with the proto phonotactics, then evolved and checked
again as a modern word.

## Stress

**Always on the second-to-last syllable** (penultimate), like most
Portuguese words. A one-syllable word carries its own stress.

There are no accent marks, so stress has to be predictable: no exceptions.

    hoida  → HOI-da
    naraxu → na-RA-xu
    hoidah → HOI-dah    (adding -h does not add a syllable)

In IPA the stress mark `ˈ` goes before the stressed syllable, in place of
the syllable dot: `naraxu` /naˈɾa.ʃu/. One-syllable words get no mark (`lu`
/lu/), because a mark would tell you nothing.
