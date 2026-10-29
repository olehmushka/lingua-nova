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
| Copula | **Always `ihar`** "be", conjugated like any verb (Portuguese *é*, *era*) |
| Prepositions | **Before the noun**; the phrase follows the object (S V O PP) |
| Subordinate clauses | **`itu` + clause**, never left out (Portuguese *que*) |
| Word formation | **Proto-level** suffixes (diminutive, augmentative, adjective, deverbal noun) and head-first compounds |

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

Several adjectives can follow in **any order**: `lu huro gumo bujo` and
`lu huro bujo gumo` both mean "the big black dog".

## Word formation (draft)

New words are built from old ones **in the proto-language**: the suffix or
second root is added to the proto form, and the sound changes then apply to
the whole word. So a derived word shows the same alternations as any other:
*\*nesca* + *-io* → *\*nescio* → **nescho**, with the palatal of rule 1.
Derivation is a lexicon matter: the dictionary lists each derived word, and
its `notes` say how it was made. The analyzer does not take these words apart.

The **stem** is the proto root without its final vowel: *\*sersebo* →
*serseb-*. An *-ia/-io* root keeps its *i* (*\*nuzio* → *nuzi-*), and before
a suffix that starts with *i* the two *i*'s merge. A root ending in a
consonant is its own stem (*\*ilcun*).

| Process | Proto suffix | Meaning | Example |
|---|---|---|---|
| Diminutive | **-itto** (m), **-itta** (f) | a smaller kind of X; the young of X | lion *\*sersebo* → leopard *\*sersebitto* → **sersebito**; dog → puppy **hurito** |
| Augmentative | **-aron** (m), **-aran** (f) | a bigger kind of X | crow *\*crofe* → raven **crofaro**; pig *\*gorsu* → boar *\*gorsaron* → **gorsaro** |
| Adjective | **-io** | made of X, X-coloured | gold *\*ilcun* → golden *\*ilcunio* → **ilcunho** |
| Deverbal noun | infinitive minus *-r*, + **-tia** | the feeling or act of V-ing | hate *\*povisar* → hatred *\*povisatia* → **povisacha** |
| Compound | head root + modifier root, one word | an X that is (or has to do with) Y | horse + river → hippopotamus **brecotrubo**; cat + black → panther **nujobujo** |

- The suffixes are chosen to show the sound changes at work: *-itto* keeps a
  voiceless *t* thanks to the geminate (rule 3), *-aron* hides a lost *n*
  (rule 4), *-io* and *-tia* palatalize (rule 1).
- Diminutives and augmentatives keep the gender of their base. Deverbal
  nouns are feminine.
- **Compounds** put the head first, like noun + adjective in a phrase
  (*brecotrubo* "horse-river"). The compound takes the **gender of the
  head**, even when its last root ends in the other gender's vowel:
  fish *\*nauca* (f) + cat *\*nuzio* → catfish **nauganujo** is feminine. If the
  modifier starts with a vowel, the head loses its final vowel; a final
  *n/m* of the head is dropped (proto codas are only *r l s*).

## Possession (draft)

Like English *'s*: **possessor + -s + possessed thing**. After a consonant or plural -h, use **-es**.

| | Example | Like English |
|---|---|---|
| after a vowel | `lu mena`**`s`**` hoida` | the woman's house |
| after a consonant | `lu zahor`**`es`**` tilho` | the sea's colour |
| plural possessor | `lu menah`**`es`**` hoida` | the women's house |

As in English, the article belongs to the possessor: `lu menas hoida` = "[the woman]'s house".
Possessive pronouns also come before the noun: `me hoida` = "my house".

There is **no word for "of"**: English *X of Y* is said as *Y's X*.
`lu dauzos orloh` = "the children of the king" (= "the king's children").

The **-s goes on the possessor noun**, even when adjectives follow it:
`lu mena`**`s`**` lirma huro` = "the white woman's dog" (not *lu mena lirmas huro*).


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
- After a **preposition** a pronoun takes the **object** form too, as in
  English *with him*: `Hana sile gu honi.` = "She sleeps with him." So the
  `-i` form is really the general **non-subject** (oblique) form.
  Portuguese has special forms here instead (*para mim*, fused *comigo*).

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
- **Double negation** is required with negative words **after the verb**, as in *Não vi nada*:
  `Na naraxu zunhu.` = "I saw nothing."
- A negative word **before the verb** needs no **na**, as in Portuguese *Nada caiu*:
  `Zunhu zuvixe.` = "Nothing fell."
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
- **Question words don't inflect**: `ifa` is who, whom and whose, like
  Portuguese *quem*. As the subject it takes a **3sg** verb:
  `Ifa naraxe lu dubroh?` = "Who saw the men?" With no object, context
  decides: `Ifa naraxis?` = "Whom didst thou see?" (subject dropped), not
  "Who saw?" (that would be `Ifa naraxe?`).
- **Answering yes**: repeat the verb, as Portuguese does (*Viste? — Vi.*):
  `Naraxis honi? — Naraxu.` = "Did you see him? — Yes (I saw)."
- No answer: `Na.` or `Na naraxu.`

## Copula (draft)

**`ihar`** "be" is always there, in every tense, conjugated like any
class I verb (`iha`, `ihaxe`, `ihare`), as in Portuguese (*A espada é
longa*), not left out as in Russian (*Меч длинный*). What follows it is
an adjective (invariable, as always) or a noun phrase:

- `Lu gurzos halto iha estra.` = "The warrior's sword is long."
- `Lu dauzo ihaxe ne gurzo.` = "The king was a warrior."

A pronoun after `ihar` is in the **subject** form, as in Portuguese
(*Sou eu*) and unlike colloquial English (*It's me*):

- `Hunu iha mu.` = "It is I."

## Prepositions (draft)

Prepositions come **before** their noun phrase, as in English and
Portuguese. That's the usual pattern for a verb–object language like
Rifatian (Greenberg's word-order universals: VO languages tend to have
prepositions, OV languages postpositions). The prepositional phrase
comes **after the object**: S V O PP.

| | | |
|---|---|---|
| `bi` | in, at | `Lu orloh sileh bi lu hoida.` = "The children sleep in the house." |
| `epe` | to | `Lu mena hibaxe gurna epe lu orlo.` = "The woman gave bread to the child." |
| `gu` | with | |
| `fai` | over, above, across | `Lu frinhah letah fai lu velda.` = "The birds fly over the mountain." |

The article stays a separate word after a preposition (`bi lu hoida`):
there are no contractions like Portuguese *em + a = na*.
An indirect object ("to the child") is a phrase with `epe`.
A pronoun after a preposition is in the object form (see Pronouns):
`Lu mena hibaxe gurna epe mi.` = "The woman gave bread to me."
English can also put the recipient first without "to" ("gave me
bread", the *dative alternation*). Rifatian has only the frame with
`epe`, so both English sentences translate the same way.

## Subordinate clauses (draft)

A clause that is the object of a verb ("I think *that…*") starts with
**`itu`**, the same word as the question word `itu` "what". Portuguese
*que* (from Latin *quid*, "what") went the same way. English and German
took the demonstrative instead (*that*, *dass*). The clause keeps
normal word order and is **never left out**, as in Portuguese (*Acho
que…*) and unlike English ("I think the river is cold"):

- `Mu sozu itu lu trubo iha grepo.` = "I think that the river is cold."

`itu` right after a verb is the complementizer (glossed COMP); at the
start of a sentence it's the question word: `Itu lu mena beuzexe?` =
"What did the woman find?"

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

### 4. "She sleeps with him."

```
Hana   sil-e         gu    honi.
3SG.F  sleep-TH.3SG  with  3SG.M.OBJ
"She sleeps with him."
```

- Stress: HA-na SI-le gu HO-ni.
- `silir` is class III: present 3sg `sile`.
- After a preposition the pronoun is in the object form, `honi`, not
  `hono`, as English says *with him*, not *with he*.

## Open questions

- Extra tenses (perfect, conditional, subjunctive): **postponed**. Only
  present, past and future until after Phase 4 (morphology generator).
- Whether a prepositional phrase can come first (`Bi lu hoida, …`): not
  decided. (A pronoun after a preposition is in the object form: decided
  2026-10-25.)
- Relative clauses ("the dog *that* ate the bread"): not decided; `itu`
  is only the complementizer for now.
- "X of Y" with an indefinite or demonstrative X ("*a* child of the
  king", "*this* son of the king"): `lu dauzos orlo` has room for one
  article only, the possessor's. Not decided.
