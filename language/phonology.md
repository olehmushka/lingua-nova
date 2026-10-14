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
- **`l`, `n`, `c` are never followed by `h`**, in proto or modern words.
  `lh nh ch` are digraphs, so l+/x/ would be spelled the same as /ʎ/
  (`bilha`: bi-lha or bil-ha?). Banning the sequence keeps every spelling
  readable in one way only. The validator enforces it.

## Stress

**Always on the second-to-last syllable** (penultimate), like most
Portuguese words. A one-syllable word carries its own stress.

There are no accent marks, so stress has to be predictable: no exceptions.

    hoida  → HOI-da
    naraxu → na-RA-xu
    hoidah → HOI-dah    (adding -h does not add a syllable)
