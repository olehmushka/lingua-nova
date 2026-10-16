# Rifatian grammar

Items marked **(draft)** are placeholder forms, not settled yet. The structure
is decided; the exact sounds can still change.

## Overview

| Feature | Decision |
|---|---|
| Word order | **SVO** (subject–verb–object), like English and Portuguese |
| Articles | Definite + indefinite, **invariable**, before the noun, used like English |
| Gender | **Two genders, no agreement**: visible only in noun endings and 3rd-person pronouns |
| Plural | A **suffix on the noun** (-h / -eh); articles and adjectives don't change |
| Adjectives | **After the noun**; invariable |
| Possession | **English-style**: possessor first, marked with -s (like *'s*) |
| Case | None on nouns; pronouns have subject / object / possessive forms |
| Verbs | **Romance-style conjugation, 3 classes**; endings for person and tense; subject pronouns can be dropped |
| Negation | **Portuguese-style**: particle before the verb, double negation |
| Questions | **Portuguese-style**: same order as statements, rising intonation |

The inflection tables below also exist as data in `morphology.yaml`
(`lnova inflect <id>` prints a word's paradigm). Keep the two in sync.

## Nouns

### Gender

- Nouns ending in **-a** are typically feminine; nouns ending in **-o** are typically masculine.
- Other endings: the gender is listed in the dictionary.
- Gender **never changes other words**. It only decides which 3rd-person pronoun (he/she) refers to the noun.

### Plural (draft)

A Sumerian-flavored suffix using /x/:

| After | Suffix | Example |
|---|---|---|
| a vowel | **-h** | hoida → hoidah |
| a consonant | **-eh** | zahor → zahoreh |

Stress stays where it was: HOI-da → HOI-dah.

The same **-h** marks plural on verbs (3pl) and on pronouns (we, you all, they),
so it's one recognizable "plural sound" across the language.

## Articles (draft forms)

| | Form | Used like |
|---|---|---|
| Definite | **lu** | English *the* |
| Indefinite | **ne** | English *a/an* (singular only) |

Articles never change: `lu hoida`, `lu hoidah`.

## Adjectives

They come after the noun and never change: `lu hoida lirma`, `lu hoidah lirma`.

## Possession (draft)

Like English *'s*: **possessor + -s + possessed thing**. After a consonant or plural -h, use **-es**.

| | Example | Like English |
|---|---|---|
| after a vowel | `lu mena`**`s`**` hoida` | the woman's house |
| after a consonant | `lu zahor`**`es`**` tilho` | the sea's colour |
| plural possessor | `lu menah`**`es`**` hoida` | the women's house |

As in English, the article belongs to the possessor: `lu menas hoida` = "[the woman]'s house".
Possessive pronouns also come before the noun: `me hoida` = "my house".


## Pronouns (draft forms)

Built like a small agglutinative system: **stem + vowel**, with
subject **-u/-o/-a**, object **-i**, possessive **-e**, and plural **-h**.

| | Subject | Object | Possessive | Notes |
|---|---|---|---|---|
| I | **mu** | **mi** | **me** | |
| thou (informal, like *tu*) | **chu** | **chi** | **che** | |
| you (polite, like *você*) | **sela** | **sela** | **selas** | originally a noun ("honour"); takes **3sg** verbs |
| he | **hono** | **honi** | **hone** | |
| she | **hana** | **hani** | **hane** | |
| it | **hunu** | **huni** | **hune** | |
| we | **muh** | **mih** | **meh** | |
| you all (informal) | **chuh** | **chih** | **cheh** | |
| you all (polite, like *vocês*) | **selah** | **selah** | **selahes** | plural of **sela**; takes **3pl** verbs |
| they (masc./mixed, like *eles*) | **honoh** | **honih** | **honeh** | |
| they (fem., like *elas*) | **hanah** | **hanih** | **haneh** | |

- The polite **sela** behaves like a noun, as *você* (from *vossa mercê*,
  "your grace") does in Portuguese: 3rd-person verb, possessive with **-s**.
  Its plural is the regular noun plural **selah**, with a 3pl verb, like
  Brazilian *vocês*: `Selah narah?` = "Do you (all, polite) see?"
- **it** (`hunu`) has no plural. Things in the plural are `honoh`, like
  Portuguese *eles* for masculine things.
- Object pronouns come **after the verb**, in normal SVO position:
  `Mu naraxu chi.` = "I saw thee." (Unlike Portuguese clitics, they never move.)

## Verbs (draft)

### Three classes

The class is shown by the **theme vowel** in the infinitive, as in Portuguese -ar/-er/-ir:

| Class | Infinitive | Example |
|---|---|---|
| I | -**ar** | narar "to see" |
| II | -**er** | gilver "to know" |
| III | -**ir** | silir "to sleep" |

### Present

Ending = **theme vowel + person ending**. The 1sg has no theme vowel, like Portuguese *falo/como/parto*.

| Person | Pattern | I narar | II gilver | III silir |
|---|---|---|---|---|
| 1sg | -u | nar**u** | gilv**u** | sil**u** |
| 2sg | -Vs | nar**as** | gilv**es** | sil**es** |
| 3sg | -V | nar**a** | gilv**e** | sil**e** |
| 1pl | -Vmu | nar**amu** | gilv**emu** | sil**imu** |
| 2pl | -Vdi | nar**adi** | gilv**edi** | sil**idi** |
| 3pl | -Vh | nar**ah** | gilv**eh** | sil**eh** |

Class III uses **e** in the singular and 3pl but **i** in 1pl/2pl. Portuguese
does the same (*partes, parte* vs. *partimos*). It's a small naturalistic quirk.

The 3pl is the 3sg plus the plural **-h**: `nara` "sees" → `narah` "they see".

### Past: stem + theme vowel + **x** + endings

Endings: **-u -is -e -emu -edi -eh**, the same for all classes.

| | I | II | III |
|---|---|---|---|
| 1sg | nara**x**u | gilve**x**u | sili**x**u |
| 3sg | nara**x**e | gilve**x**e | sili**x**e |
| 3pl | nara**x**eh | gilve**x**eh | sili**x**eh |

### Future: infinitive + the same endings

As in Romance history (Latin *cantare habeo* → Portuguese *cantarei*):

| | I | II | III |
|---|---|---|---|
| 1sg | narar**u** | gilver**u** | silir**u** |
| 3sg | narar**e** | gilver**e** | silir**e** |
| 3pl | narar**eh** | gilver**eh** | silir**eh** |

### Notes

- Subject pronouns are optional (pro-drop): `Naraxu chi.` = "I saw thee."
- The polite **sela** takes 3sg: `Sela naraxe?` = "Did you (polite) see?"
- Irregular verbs will come from sound changes, not from extra rules.

## Negation (Portuguese-style)

- The particle **na** (draft) goes directly **before the verb**, like *não*:
  `Mu na naraxu chi.` = "I didn't see thee."
- **Double negation** is required with negative words, as in *Não vi nada*:
  `Na naraxu zunhu.` = "I saw nothing."
- Optional emphatic final **na**, as in Brazilian *Não sei não*:
  `Na naraxu chi, na.` = "I really didn't see thee."

## Questions (Portuguese-style)

- **Yes/no questions**: same word order as statements, with rising intonation and **?** in writing:
  `Chu naraxis honi?` = "Didst thou see him?"
- **Wh-questions**: the question word comes first and nothing else moves (no *do*-support, no inversion).
  There are two series:
  - **i-** for participants and places: `ifa` who, `itu` what, `iba` where
  - **cu-** for circumstances: `cuda` when, `cuxa` how, `cupe` why

  Example: `Cuda chu naraxis hani?` = "When didst thou see her?"
- **Answering yes**: repeat the verb, as Portuguese does (*Viste? — Vi.*):
  `Naraxis honi? — Naraxu.` = "Did you see him? — Yes (I saw)."
- No answer: `Na.` or `Na naraxu.`

## Example sentences

```
Mu  na   nar-a-x-u        chi.
1SG NEG  see-TH-PST-1SG   2SG.OBJ
"I didn't see thee."

Hana  nar-a     me       hoida.
3SG.F see-TH.3SG 1SG.POSS house
"She sees my house."
```

Glossing convention: every verb form is **stem-TH-TENSE-PERSON**. The
present has no tense marker, the past uses **-x-**, and the future uses
**-r-**: the old infinitive *-r*, reanalysed as a tense marker in the same slot as
*-x-*. When the theme vowel and the person ending are one sound (present 3sg
`nar-a`), the gloss joins them with a dot: `TH.3SG`.

### 1. "The woman sees the river."

```
Lu   mena   nar-a       lu   trubo.
the  woman  see-TH.3SG  the  river
"The woman sees the river."
```

- Stress: lu ME-na NA-ra lu TRU-bo.
- Present 3sg = stem + theme vowel; class I `narar` → `nara`.
- `mena` is feminine and `trubo` masculine, but nothing agrees, so here
  gender is invisible.

### 2. "The men will drink the water."

```
Lu   dubro-h  pulv-a-r-eh       lu   exa.
the  man-PL   drink-TH-FUT-3PL  the  water
"The men will drink the water."
```

- Stress: lu DU-broh pul-VA-reh lu E-xa. `br` is a legal onset, so
  `du.broh`; plural `-h` adds no syllable.
- `pulvar` is class I (-ar). The future is the infinitive `pulvar` + the
  shared endings (`-eh` for 3pl).
- The article stays `lu` in the plural. Only the noun takes `-h`.

### 3. "Didst thou see the woman's dog?"

```
Chu  nar-a-x-is      lu   mena-s      huro?
2SG  see-TH-PST-2SG  the  woman-POSS  dog
"Didst thou see the woman's dog?"
```

- Stress: CHU na-RA-xis lu ME-nas HU-ro.
- Yes/no question: same order as a statement, rising intonation, `?`.
- Possession: `-s` after a vowel-final possessor. `lu` belongs to the
  possessor: [lu mena]-s huro, "[the woman]'s dog".
- Pro-drop variant: `Naraxis lu menas huro?` Answer "yes": `Naraxu.`

## Open questions

- Extra tenses (perfect, conditional, subjunctive): **postponed**. Only
  present, past and future until after Phase 4 (morphology generator).
