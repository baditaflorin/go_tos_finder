# Changelog

All notable changes to this service are recorded here, newest first.

## 1.8.0 — 2026-08-23

### Added

EU-market-expansion real-evidence round 27: Malta — the LAST of the 13
target EU countries for this multi-round expansion (rounds 15-27 now
cover all of Romania, Czechia, Hungary, Greece, Bulgaria, Croatia,
Slovakia, Slovenia, Cyprus, Lithuania, Latvia, Estonia, and Malta, on top
of the 15 countries covered by the 14 prior rounds). Fetched a real, live
art gallery/e-commerce site's Terms of Sale page (artemisialtd.com, naming
Artemisia Fine Arts & Antiques Limited) and ran the shipped 1.7.21
extractor against it: Present=true but every other field empty
(CompletenessScore=0). Found and fixed three compounding real bugs:

- **Malta's VAT format uses an optional dash between the two 4-digit
  halves** ("MT2336-6423") — the existing bare-adjacent `\bMT\d{8}\b`
  pattern never matched it. Loosened to `\bMT\d{4}-?\d{4}\b`.
- **Malta's "Company Registration Number" (Malta Business Registry's own
  "C"+4-6-digits convention, e.g. "C 71943") had no identifier pattern at
  all.** Added, anchored on the English label text (the bare "C NNNNN"
  shape alone would be dangerously generic to match unlabelled anywhere
  in text) — wired Register-only (same "format-match, no invented
  checksum" discipline as Ireland's CRO/Croatia's MBS) and connected to
  `singleCountryIdentifierKind` so it self-heals Country away from the
  suffix-guessed "GB" (a bare "Limited" suffix collides with
  suffixTable's existing GB entry) to the correct "MT".
- **UK/Malta-style same-line "&lt;Name&gt; (&lt;number&gt;) of &lt;address&gt;
  (\"&lt;nickname&gt;\")" drafting was invisible to address extraction** —
  `extractAddressNearEntity`'s forward per-line scan only ever looks at
  lines AFTER the one naming the entity, so an address on the SAME line
  as the name was never seen. Added a same-line inline extraction
  (`inlineOfAddressRE`) anchored to the "of " token immediately following
  the name/company-number clause, capturing up to the next "(" (which
  always opens the nickname clause in this drafting style).

Also fixed `formatRegister` to strip a leading "is "/"Is " token, since
this Kind's raw identifier match deliberately keeps the optional "is"
inside it (`findIdentifiers` needs the RAW match for proximity-attachment;
cleanup happens only at display time).

One honestly-documented non-fix: no VAT checksum algorithm was
independently verified against a real fetched value, so VATValidation
stays "format_valid" (not "checksum_valid") — same discipline as
Ireland/Croatia/Slovakia/Cyprus/Luxembourg/Latvia before it. Also NOT
matched: the page's other, bare-parenthetical restatement of the same
company number, "(C 71943)" directly after the entity name with no label
at all — deliberately excluded as too generic to match unlabelled. Four
new regression tests (imprint_extract_eu_round27_test.go); full existing
suite green twice in a row, no regressions. No dedicated Maltese ruleset
added — consistent with prior rounds' discipline.

## 1.7.21 — 2026-08-23

### Fixed

EU-market-expansion real-evidence round 26: Estonia. Fetched a real, live
garden-supplies e-shop's Müügitingimused page (voluaed.ee, naming OÜ Iru
Aiakeskus) and ran the shipped 1.7.20 extractor against it. Entity name,
suffix, country, and VAT already extracted correctly — "OÜ" is an existing
high-confidence suffixTable entry, and the pre-existing bare
"EE"-prefixed EU VAT pattern already matched "KMKR nr: EE101490959"
without needing a label anchor. Found and fixed two compounding real bugs:

- **Estonia's "registrikood" (company registry code) had no identifier
  pattern at all.** Added it, wired as Register-only — a genuinely
  separate 8-digit number from Estonia's own 9-digit KMKR nr (VAT
  number), same architecture as Lithuania's Įmonės kodas/Latvia's split
  (not Bulgaria/Greece/Croatia's shared-body architecture). No dedicated
  checksum implemented this round.
- **The "KMKR nr: EE101490959" line leaked into Address** (sits right
  after the entity's own line) — added "kmkr"/"registrikood" as
  skip-past markers.

One real, honestly-documented gap NOT fixed: Address stays empty — the
real address sits on the SAME line as the entity name and its
registrikood parenthetical, the same class of gap already left
undocumented for Romania/Croatia/Slovenia/Lithuania. Also honestly
note-worthy (not a bug): LegalName includes the source's own parenthetical
register annotation verbatim ("OÜ Iru Aiakeskus (registrikood 12178134)")
— a faithful quote of the real page, left as-is since the register number
is already separately and correctly captured in Register regardless.
Three new regression tests (imprint_extract_eu_round26_test.go); full
existing suite still green, no regressions. No dedicated Estonian ruleset
added — consistent with prior rounds' discipline.

## 1.7.20 — 2026-08-23

### Fixed

EU-market-expansion real-evidence round 25: Latvia. Fetched a real, live
household-goods e-shop's Juridiskā informācija page (gatavosana.lv, naming
SIA SOLLER.LV) and ran the shipped 1.7.19 extractor against it — it
extracted NOTHING at all (CompletenessScore 0), despite "SIA" being an
existing, already-high-confidence suffixTable entry. Found and fixed three
compounding real bugs:

- **Latvia's "Reģistrācijas numurs" (registration number) had no
  identifier pattern at all.** Added it as VAT-equivalent (its 11-digit
  body IS the same number the "LV"-prefixed EU VAT pattern matches — the
  same architecture as Bulgaria's ЕИК/Greece's ΑΦΜ/Croatia's OIB, unlike
  round 24's Lithuanian Įmonės kodas, which is genuinely separate). No
  checksum implemented (stays `format_valid` — several plausible weight
  sequences were tried by hand against the one real value and none
  matched, so nothing was guessed).
- **A general bug in `extractEntityAfterSuffix`** (the prefix-form name
  extractor round 24 introduced): this page's real entity name,
  "SIA SOLLER.LV", is a domain-style brand name containing its own period.
  The "stop at the first period" rule truncated it to just "SOLLER",
  dropping the genuine ".LV" suffix. Added a narrow structural check
  (`looksLikeDomainSuffixPeriod`): a period immediately followed by 2-4
  uppercase letters and then a non-letter reads as a domain suffix, not a
  sentence boundary — a genuine sentence-ending period still stops the
  scan exactly as before.
- **"Reģistrācijas numurs"/"PVN maksātāja numurs" values leaked into
  Address** (both sit right before the real "Juridiskā adrese:" line) —
  added both as skip-past markers.

Three new regression tests (imprint_extract_eu_round25_test.go); full
existing suite still green, no regressions. No dedicated Latvian ruleset
added — consistent with prior rounds' discipline.

## 1.7.19 — 2026-08-23

### Fixed

EU-market-expansion real-evidence round 24: Lithuania. Fetched a real, live
group-buying e-shop's Taisyklės page (grupinispirkimas.lt, naming UAB
"GRUPINIS PIRKIMAS" in Lithuania's prefix-form convention — legal form
before the quoted trading name) and ran the shipped 1.7.18 extractor
against it — it extracted NOTHING at all (CompletenessScore 0), despite
"UAB" being an existing, already-high-confidence suffixTable entry. Found
and fixed six compounding real bugs:

- **Lithuania's "Įmonės kodas" (legal-entity code) had no identifier
  pattern at all.** Added it as Register-only (a genuinely separate number
  from Lithuania's own PVM kodas/VAT code, unlike Bulgaria's ЕИК/Greece's
  ΑΦΜ/Croatia's OIB), plus a new `ltCompanyCodeValid` two-pass
  weighted-mod-11 checksum confirmed against two independent real values.
- **The pattern needed no leading `\b`** (round 18/19's RE2 lesson,
  confirmed non-script-specific: "Į" is Latin Extended-A).
- **A literal un-decoded "&nbsp;"** sits between the label's colon and the
  digits on this real page — added an optional-`&nbsp;` tolerance.
- **`extractEntityAround`'s prefix-form fallback was gated on the ENTIRE
  preceding text being blank**, but this real page has "Pardavėjas
  &#8211; UAB „...\"" (a role label plus dash) preceding the suffix, so
  the fallback never even tried. Widened the gate to also fire when the
  preceding text ends in a dash-style separator (Unicode dashes and their
  common un-decoded HTML-entity spellings).
- **The prefix-form name itself is quote-wrapped with un-decoded numeric
  entities** ("&#8222;GRUPINIS PIRKIMAS&#8221;") — `cleanCandidateName`'s
  entity-corruption filter was rejecting this well-formed quoted name
  outright. Added a new `stripQuoteDelimiters` helper that trims a known
  quote-delimiter pair before the corruption check ever sees it.
- **`formatRegister` leaked the same "&nbsp;" straight into the displayed
  Register string** — added explicit `&nbsp;`-prefix stripping to its
  existing cleanup pass.

One real, honestly-documented gap NOT fixed: Address stays empty — the
real address sits on the SAME line as the entity name and its Įmonės
kodas/PVM kodas clause, the same class of gap already left undocumented
for Romania/Croatia/Slovenia. No checksum implemented for Lithuania's VAT
either (stays `format_valid`). Four new regression tests
(imprint_extract_eu_round24_test.go); full existing suite still green, no
regressions. No dedicated Lithuanian ruleset added — consistent with prior
rounds' discipline.

## 1.7.18 — 2026-08-23

### Fixed

EU-market-expansion real-evidence round 23: Cyprus. Fetched a real, live
electronics retailer's eStore Terms & Conditions page (epic.com.cy, naming
epic ltd — English-language text, a legacy of Cyprus's common-law
tradition) and ran the shipped 1.7.17 extractor against it — it extracted
NOTHING at all (CompletenessScore 0). Found and fixed six compounding real
bugs, an unusually dense round:

- **Cyprus's "HE" (Registrar of Companies file-number) and domestic VAT
  number had no identifier patterns at all.** Added "HE" (Register-only)
  and a new "VAT Number" Kind (VAT-equivalent, gated on the English "VAT
  ... number" phrase plus a digit+letter shape that is itself
  distinctively Cypriot) with its own dedicated cleaner — reusing the bare
  "VAT" Kind would have corrupted its generic cleanup for every other
  country's already-clean hits, since this match necessarily swallows its
  own anchoring label text.
- **The real entity ("epic ltd") uses an all-lowercase brand stylization**
  that suffixTable's existing capitalised "Ltd"/"Ltd." entries never
  matched. Added lowercase "ltd" — same shape as the pre-existing
  "PLC"/"plc" pair already in this table.
- **The real T&C paragraph states everything in ONE 526-rune sentence**
  with no `<br>` breaks — over the existing 300-rune per-line cap. Raised
  to 600.
- **`cleanCandidateName` unconditionally rejected any lowercase-initial
  candidate**, discarding this page's genuine, deliberately-lowercase
  brand name outright. Narrowed to only reject when the candidate's stem
  has 3+ words — a genuine sentence fragment needs several words to be
  grammatical; a brand-name stem is virtually always 1-2 words.
- **`extractAddressNearEntity`'s forward scan had no stop condition for a
  later line repeating the entity's own name**, so a scan triggered from
  an earlier (unsuccessful) mention kept going and absorbed the page's
  separate, cleaner footer signature whole (including the entity name
  itself) as "address" content. Added a stop check for exactly that case.

Country correctly resolves to "CY" (not the suffix-guessed "GB") because
the HE/VAT Number ground-truth identifiers override it, confirming the
same self-healing mechanism rounds 20-22 already established for other
suffix collisions. One real, honestly-documented gap NOT fixed: the T&C
paragraph's own mid-sentence mention still produces no usable candidate (an
80-char backward-scan-window limitation, the same shape as round 14's
Norway gap) — the regression test uses the page's separate, clean footer
signature instead, exactly as round 14 did. No checksum implemented for
Cypriot VAT either (stays `format_valid`). Four new regression tests
(imprint_extract_eu_round23_test.go); full existing suite still green, no
regressions. No dedicated Cypriot ruleset added — consistent with prior
rounds' discipline.

## 1.7.17 — 2026-08-23

### Fixed

EU-market-expansion real-evidence round 22: Slovenia. Fetched a real, live
retailer's Splošni pogoji page (sgermobil.si, naming SGERM trgovina,
storitve, posredništvo in proizvodnja d.o.o.) and ran the shipped 1.7.16
extractor against it — it extracted NOTHING at all (CompletenessScore 0).
This is the ACTUAL Croatia/Slovenia "d.o.o." collision round 20's
suffixTable comment anticipated. Found and fixed four compounding real
bugs:

- **Slovenia's "Davčna številka" (tax number) had no domestic pattern, and
  the existing "SI"-prefixed EU VAT pattern required zero space** — but
  this real page writes it as "Davčna številka: SI 11597267"
  (space-separated), same class of gap round 7's BE/FR fixes already
  covered. Fixed by adding the same optional-space tolerance, which alone
  was enough to unblock the whole identifier-gated extraction.
- **Slovenia's "Matična številka" (company registration number) had no
  identifier pattern at all.** Added it, wired as Register-only (same
  shape as Croatia's MBS).
- **"Matična številka"/"Davčna številka" values leaked into Address**
  (both sit right after the real "2000 Maribor" line) — added both as
  skip-past markers.
- **A separate court-registration citation sentence also leaked into
  Address** (its own insert-number digit run cleared `looksAddressLine`,
  same shape as round 10's Luxembourg RCS citation) — added "vložno
  številko" as a third skip-past marker.

"Matična številka" was also added to `singleCountryIdentifierKind`
(-> "SI") — the concrete confirmation that round 20's self-healing design
works end to end: this page's suffix-guessed country would otherwise have
been "HR" (suffixTable's only "d.o.o." entry), but the ground-truth
"Matična številka" evidence correctly overrides it to "SI", the exact
mechanism round 20 predicted before any Slovenian evidence existed to
confirm it. One real gap honestly left NOT fixed: Address only captures
"2000 Maribor" — the real street line never clears `looksAddressLine` (no
Slovenian street-word marker), same class of gap already left undocumented
for Czech/Slovak/Croatian street lines. No checksum implemented for
Slovenian VAT/Matična številka either (stays `format_valid`). Four new
regression tests (imprint_extract_eu_round22_test.go); full existing suite
still green, no regressions. No dedicated Slovenian ruleset added —
consistent with prior rounds' discipline.

## 1.7.16 — 2026-08-23

### Fixed

EU-market-expansion real-evidence round 21: Slovakia. Fetched a real, live
electrical-supplies retailer's Obchodné podmienky page (elektro-siete.sk,
naming Elektro-Siete, s.r.o.) and ran the shipped 1.7.15 extractor against
it. This round is the real test of round 16's own foresight: Czechia's
"IČO" identifier and "s.r.o." suffix are BOTH shared, letter-for-letter,
with Slovakia (former Czechoslovakia) — round 16 deliberately did NOT wire
"IČO" into `singleCountryIdentifierKind`, anticipating that a future Slovak
page should self-heal via its own SK-prefixed VAT number instead.

- **Confirmed, not fixed**: Country correctly resolves to "SK" (not the
  suffix-guessed "CZ") because this page also states "IČ DPH:
  SK2023571528" — a plain "VAT" Kind match whose "SK" prefix takes
  precedence over the suffix guess in the existing country-precedence
  chain. No code change was needed — the self-healing path worked exactly
  as round 16 anticipated. Also confirmed Slovak "IČO" uses the identical
  published check-digit algorithm as Czech "IČO" (verified by hand against
  this page's real value), so `czICOValid` needed no changes either.
- **Real bug found and fixed**: this page's "IČO:"/"DIČ:"/"IČ DPH:"/
  "IBAN:" block, plus a separate account-number line and a separate
  court-register citation, all have long digit runs that cleared
  `looksAddressLine` and got wrongly absorbed into Address —
  `extractAddressNearEntity` had no stop-marker for any of these Slovak
  labels (round 16 never needed one for its own fixture). Added
  "ičo:"/"dič:"/"ič dph"/"iban" (colon-anchored on the first two to avoid
  false-positiving on ordinary words like "dedičom") and "číslo účtu"/
  "obchodnom registri" as skip-past markers. This retroactively benefits
  round 16's Czech pages too.

One real, honestly-documented gap NOT fixed: with the pollution removed,
Address comes back empty — the real address never clears
`looksAddressLine` at all (no Slovak street-word marker, and the "941 08"
postal code's spaced digit grouping never reaches the 4-consecutive-digit
threshold), the same class of gap round 16 already left undocumented for
Czech postal codes. No checksum implemented for Slovak VAT either (stays
`format_valid` — the modern numbering scheme has been reformed over time
and this round lacked confidence in the current algorithm to verify
safely). Three new regression tests (imprint_extract_eu_round21_test.go);
full existing suite still green, no regressions. No dedicated Slovak
ruleset added — consistent with prior rounds' discipline.

## 1.7.15 — 2026-08-23

### Fixed

EU-market-expansion real-evidence round 20: Croatia. Fetched a real, live
retailer's Opći uvjeti page (mako.hr, naming Mako d.o.o.) and ran the
shipped 1.7.14 extractor against it — it extracted NOTHING at all
(CompletenessScore 0). suffixTable had zero Croatian entries of any kind.
Found and fixed four compounding real bugs:

- **No Croatian suffix entries at all.** Added "d.o.o." (real-evidence-
  confirmed) plus "j.d.o.o."/"d.d." (sibling forms, same basis as rounds
  18-19). Known, accepted collision risk documented at the entry:
  "d.o.o." is shared letter-for-letter by every former-Yugoslav-republic
  legal system including Slovenia (a later target in this series) — the
  same class of risk already tolerated for the existing "S.A."(FR/RO)/
  "S.R.L."(IT/RO) collisions.
- **Croatia's OIB (personal/legal ID) and MBS (court-register number) had
  no identifier patterns at all.** Added both — "OIB" as VAT-equivalent
  (same 11-digit body as the "HR"-prefixed EU VAT pattern), "MBS" as
  Register-only — plus a new `hrOIBValid` ISO 7064 MOD 11-10 checksum
  confirmed against the real value (31448356613). Both use plain ASCII
  letters, so no leading-`\b` RE2 gotcha applied here.
- **A literal "•" (U+2022) bullet-marker character** precedes each
  defined-term line on this real page, and `extractEntityAround`'s
  backward-scan had no stop condition for it (unlike '\n'/'\r'/'|'),
  capturing the bullet plus the whole preceding clause into the
  candidate name. Added "•" as a stop character.
- **The defining clause itself** ("Mako znači Mako d.o.o." — "[Term]
  means [Full legal name]") still duplicated the short name even with
  the bullet fixed. Added " znači " (Croatian "means") to
  `trimAtConjunction`'s marker list.

One real, honestly-documented gap NOT fixed: Address stays empty on this
page — the real address sits on the SAME line as the entity+register
clause, same class of gap as round 15's Romanian same-line address. Four
new regression tests (imprint_extract_eu_round20_test.go); full existing
suite still green, no regressions. No dedicated Croatian ruleset added —
consistent with prior rounds' discipline.

## 1.7.14 — 2026-08-23

### Fixed

EU-market-expansion real-evidence round 19: Bulgaria. Fetched a real, live
diving/sports-equipment retailer's Общи условия page (cressi.bg, naming Кеме
ЕООД in native Cyrillic script) and ran the shipped 1.7.13 extractor against
it — it extracted almost nothing (only "contact", CompletenessScore 33).
suffixTable had zero Bulgarian entries of any kind (Latin or Cyrillic).
Found and fixed three compounding real bugs, applying round 18's Greek
lessons proactively this time:

- **No Bulgarian suffix entries at all.** Added "ЕООД" (real-evidence-
  confirmed) plus "ООД"/"АД"/"ЕАД" (added alongside it on the same
  well-established-sibling-forms basis round 18 used for Greek).
- **Bulgaria's ЕИК (Unified Identification Code / BULSTAT) had no
  identifier pattern at all.** Added it, wired as VAT-equivalent (its
  9-digit body IS the same number the "BG"-prefixed EU VAT pattern
  matches), plus a new `bgEIKValid` two-pass weighted-mod-11 checksum (the
  published BULSTAT algorithm) confirmed against the real value
  (201845795). Applied round 18's RE2 lesson proactively: no leading `\b`
  on the Cyrillic-anchored pattern.
- **The identifier's own line still leaked into Address** (it sits before
  the real address line on this page) — added "еик" as a skip-past
  address marker, same shape as round 17/18's Hungarian/Greek markers.

Also confirmed: the pre-existing "BG"-prefixed VAT pattern already matched
this page's own VAT citation without any changes — "BG" is always Latin
per EU-wide VAT-number convention, so it was never subject to the
Cyrillic-`\b` gotcha bug 2 found for the domestic label. "ЕИК" was also
added to `singleCountryIdentifierKind` (-> "BG") — unlike round 18's Greek
"ΑΦΜ", Bulgaria is the only EU member using Cyrillic script officially, so
there's no sibling-country sharing risk to guard against. Four new
regression tests (imprint_extract_eu_round19_test.go); full existing suite
still green, no regressions. No dedicated Bulgarian ruleset added —
consistent with prior rounds' discipline.

## 1.7.13 — 2026-08-23

### Fixed

EU-market-expansion real-evidence round 18: Greece. Fetched a real, live
phone-accessories retailer's Όροι Χρήσης page (thikishop.gr, naming MASTER
ACCESSORIES Ι.Κ.Ε. in NATIVE Greek script) and ran the shipped 1.7.12
extractor against it — it extracted almost nothing (only "contact",
CompletenessScore 33). suffixTable already carried Latin-transliterated
Greek legal forms ("A.E.", "E.P.E.", "O.E.", "I.K.E.") with an explicit
comment flagging native Greek script as out of scope — this round's real
evidence is exactly that gap (Greek Ι/Κ/Ε are different Unicode code points
from Latin I/K/E). Found and fixed five compounding real bugs:

- **No native-Greek-script suffix entries at all.** Added "Ι.Κ.Ε."
  (real-evidence-confirmed) plus "Α.Ε."/"Ε.Π.Ε."/"Ο.Ε." (the native-script
  counterparts of the already-existing Latin quartet).
- **Greece's ΑΦΜ (tax number) and Γ.Ε.ΜΗ. (Commercial Registry number) had
  no identifier patterns at all.** Added both — "ΑΦΜ" as VAT-equivalent
  (same 9-digit body as the "EL"-prefixed EU VAT pattern, same architecture
  as Hungary's Adószám), "Γ.Ε.ΜΗ." as Register-only — plus a new
  `grVATValid` weighted-mod-11 checksum confirmed against the real value
  (800617296).
- **A general Go regexp gotcha**: RE2's `\b` is ASCII-only and never
  recognises a Greek (or any non-ASCII) letter as a "word" character, so a
  naive `\bΑΦΜ\b`-style pattern can never match anywhere. Every earlier
  non-ASCII pattern (IČO, Adószám, Cégjegyzékszám) happened to start/end on
  a plain ASCII character, so this never surfaced before. Fixed by dropping
  the leading `\b` on both new Greek patterns.
- **Label+value pairs split across a tag-boundary newline** (the
  `<strong>ΑΦΜ:</strong> 800617296` shape) — a new variant of round 8's
  Portuguese finding and round 16's Czech suffix-split, this time a colon
  immediately followed by a digit across the break. Added
  `labelColonDigitBoundaryRE`, narrowly scoped so a genuine section-heading
  break stays untouched. Also fixed a related leak: the matched
  identifier's raw Value (needed byte-exact for the proximity-lookup) was
  carrying the embedded newline through to `formatRegister`'s output —
  fixed by collapsing whitespace at display time instead of at match time.
- **Both identifier lines still leaked into Address** even once reachable
  on one line — added "αφμ"/"γ.ε.μη." as skip-past address markers, same
  shape as round 17's Hungarian markers.

Deliberately NOT added to `singleCountryIdentifierKind`: "ΑΦΜ" — Greek is
also official in Cyprus, so (unlike "Γ.Ε.ΜΗ.", a named Greek-only
institution) it's treated with the same Czech/Slovak "IČO" caution from
round 16. Five new regression tests (imprint_extract_eu_round18_test.go);
full existing suite still green, no regressions. No dedicated Greek
ruleset added — consistent with prior rounds' discipline.

## 1.7.12 — 2026-08-22

### Fixed

EU-market-expansion real-evidence round 17: Hungary. Fetched a real, live
specialty-foods e-shop's Impresszum page (szatmari-izek.shop.hu, naming
Szatmári Ízek Kft.) and ran the shipped 1.7.11 extractor against it — it
extracted almost nothing (only "contact", CompletenessScore 33), despite
"Szatmári Ízek Kft." being trivially suffix-matchable ("Kft.", an existing
native HU suffixTable entry) right next to its register number and tax
number. Found and fixed four compounding real bugs:

- **isLegalPage doesn't recognise the Hungarian "impresszum"**: it's NOT a
  substring of the English "impressum" the gate already checked for (the
  Hungarian doubles "sz" where German doubles "s") — same failure shape as
  round 14's Dutch "colofon" gap. Added "impresszum" to the keyword list.
- **"Név: " (Hungarian "Name:") label leaked into LegalName**: no Hungarian
  entry existed in stripLabelPrefix's list. Added it.
- **Hungary's Adószám and Cégjegyzékszám had no identifier patterns at
  all**, so `findIdentifiers` found nothing and (combined with the first
  bug) the whole suffix-anchored scan never ran. Added dedicated patterns
  for both: "Cégjegyzékszám" (company-register number) wired as
  Register-only (same shape as round 16's Czech IČO); "Adószám" (domestic
  tax number) wired as VAT-equivalent (its 8-digit core IS the same number
  the "HU"-prefixed EU VAT pattern matches, same shape as Poland's NIP),
  plus a new `huAdoszamValid` weighted-mod-10 checksum confirmed against
  TWO independent real values on the same page (13495413 and 23495919).
- **Both new identifier lines leaked into Address**: the real page's
  "Cégjegyzékszám: ...<br />Adószám: ...<br />Székhely: ..." sequence has
  the real address AFTER both identifier lines — extractAddressNearEntity
  had no skip marker for either, same failure shape as round 5's Polish
  NIP/KRS/REGON. Added both as skip-past (not stop) markers.

Both "Cégjegyzékszám" and "Adószám" also added to
`singleCountryIdentifierKind` (-> "HU") — genuinely single-country, unlike
round 16's Czech "IČO" (deliberately excluded there since it's shared with
Slovakia). Three new regression tests (imprint_extract_eu_round17_test.go);
full existing suite still green, no regressions. No dedicated Hungarian
ruleset added — consistent with round 15/16's discipline.

## 1.7.11 — 2026-08-22

### Fixed

EU-market-expansion real-evidence round 16: Czechia. Fetched a real, live
white-goods e-shop's Obchodní podmínky page (onlineshop.cz, naming SHOP
TRADING, s.r.o.) and ran the shipped 1.7.10 extractor against it — it
extracted NOTHING at all (CompletenessScore 0), despite "SHOP TRADING,
s.r.o." being trivially suffix-matchable ("s.r.o.", an existing native
CZ-unique suffixTable entry) right next to its IČO. Found and fixed three
compounding real bugs:

- **stripTagsLines' own tag-boundary newline split "s.r.o." in two**: this
  page writes the entity name as `<strong>SHOP TRADING, s.r.o</strong>., se
  sídlem ...` — a common real pattern (bold name, plain trailing
  punctuation outside the tag) — so the closing "." of the suffix landed on
  its own line, defeating suffixTable's literal dot-terminated match
  entirely and silently emptying the whole suffix-anchored scan. Fixed with
  a new `tagBoundaryPunctRE` post-process step: collapses a tag-boundary
  newline back out only when a Unicode letter immediately precedes it and a
  bare "."/"," immediately follows (mid-abbreviation split), NOT when a
  digit precedes it. The digit gate was necessary after an unconditional
  first attempt caused a real regression: eurostarshotels.com's real aviso
  legal (round 2) has an isolated phone number in its own `<a href="tel:...">`
  tag, and unconditionally collapsing merged it into the following
  sentence, leaking it into the address.
- **The exposed real sentence was 280 runes but 307 bytes** (Czech
  diacritics are 2 bytes each in UTF-8) — over extractImprintText's
  existing 220-byte per-line cap (`len()`, raised from 140 in round 2 for
  the same class of gap). Switched to `utf8.RuneCountInString` so the cap
  means what it says regardless of script, raised to 300.
- **Czechia's IČO had no identifier pattern at all**, so `findIdentifiers`
  found nothing and (since the real URL has no English isLegalPage
  keyword either) the whole function bailed out before the suffix scan
  could run. Added a dedicated "IČO" vatPattern, wired into the
  Register-field Kind switch (same silently-dropped-Kind shape as round
  14/15's OrgNr/CUI), and a new `czICOValid` weighted-mod-11 checksum,
  confirmed against this page's real IČO (24717509).

Deliberately NOT added: an "IČO" case in `singleCountryIdentifierKind` —
unlike CUI/KvK/CRO/CVR/Y-tunnus/OrgNr, "IČO" is shared with Slovakia
(former Czechoslovakia), so mapping it to "CZ" would risk mis-flagging a
future real Slovak page; not needed for this fixture since "s.r.o." is
already CZ-unambiguous. One real, honestly-documented gap NOT fixed:
Address stays empty on this page — the real address sits on the SAME line
as the entity+register clause (no further tag boundary to split it out),
and a separate, cleaner section elsewhere on the page doesn't clear
looksAddressLine either (no Czech street-word marker, and Czech postal
codes' "XXX XX" spaced format never reaches the 4-consecutive-digit
threshold). Three new regression tests
(imprint_extract_eu_round16_test.go); full existing suite still green, no
other regressions. No dedicated Czech ruleset added — consistent with
round 15's discipline.

## 1.7.10 — 2026-08-22

### Fixed

EU-market-expansion real-evidence round 15 (new series, rounds 15+, covering
the 13 remaining EU countries): Romania. Fetched a real, live e-commerce
company's Termeni si conditii page (emag.ro, naming its operating entity
DANTE INTERNATIONAL S.A.) and ran the shipped 1.7.9 extractor against it.
Romania already had a native "S.R.L." suffix entry and a "CUI" identifier
pattern/checksum wired up since this codebase's first EU-expansion round,
but had never been real-evidence-verified end to end. Found and fixed two
concrete, compounding real bugs:

- **Country resolved to "FR" instead of "RO"**: the entity's bare "S.A."
  legal-form suffix collides with France's own low-confidence "S.A." entry
  in suffixTable, and Romania's own real "CUI: 14399840" identifier —
  correctly matched, Register correctly populated as "CUI 14399840" — never
  overrode the wrong suffix-guessed country, because
  `singleCountryIdentifierKind` (the mechanism that already lets ground-truth
  government-identifier evidence override a merely-probabilistic suffix
  guess for IT/NL/PL/PT/IE/LU/SE/DK/FI/NO) had never had a case for
  Kind="CUI" at all, despite CUI being wired into vatPatterns and the
  checksum switch since round 1. Added a "CUI" case returning "RO".
- **The site's own footer copyright line leaked into Address**: once the
  country fix surfaced the footer's real content, extractAddressNearEntity's
  forward-scan stop-marker list (which already excludes Poland's
  NIP/KRS/REGON, Denmark's CVR, the Netherlands' KvK/BTW-id, France/
  Luxembourg's RCS, and France/Belgium's TVA from address absorption) had no
  entry for Romania's own "CUI"/"CIF" label — the unrelated "© 2001-2026
  Dante International, CUI: 14399840, Reg. Com. J2002000372404" line got
  absorbed wholesale into Address. Added a word-boundary `cuiWordRE` check
  (a bare substring check would false-positive on "cui" as an ordinary
  standalone Romanian word).

One real, honestly-documented gap NOT fixed this round: the entity's own
run-on sentence (name, then address, then register clause, all in one
sentence with no punctuation/tag boundary between them) still over-captures
the register text into Address on this specific page — a same-line
entity-boundary issue, a different code path from the two fixes above.
Three new regression tests (imprint_extract_eu_round15_test.go); full
existing suite still green, no regressions. No dedicated Romanian ruleset
added: this fixture didn't establish a hard, unconditional register-number
requirement distinct from the EU baseline — consistent with the discipline
of rounds 7-14 (Belgium/Portugal/Ireland/Luxembourg/Sweden/Denmark/Finland/
Norway).

## 1.7.9 — 2026-08-22

### Fixed

EU-market-expansion real-evidence round 14 (final round of this series):
Norway. Fetched a real, live photo-printing webshop's Kjøpsvilkår
(terms-of-purchase) page (japanphoto.no, naming CEWE Norge AS) and ran the
shipped 1.7.8 extractor against it — it extracted NOTHING at all
(CompletenessScore 0), despite "CEWE Norge AS" being trivially
suffix-matchable ("AS") right next to its org.nr. Found and fixed four
compounding real bugs, all affecting the same real Norwegian identifier:

- **The "OrgNr" vatPattern was case-sensitive**, requiring a literal
  capital "Org" — but ordinary mid-sentence Norwegian prose lower-cases it
  ("...fra CEWE Norge AS med org.nr. 965 321 039..."). Made the pattern
  case-insensitive.
- **"OrgNr" had never been listed in extractImprintText's Kind switch at
  all** — present in the vatPatterns table since this codebase's very
  first EU-expansion round, but silently dropped every single time it
  matched. Added "OrgNr" to the Register-field case.
- **Norway's organisasjonsnummer had no checksum-validation case at all**,
  despite the published Brønnøysundregistrene weighted-mod-11 algorithm
  being well documented — it always fell through to formatValid. Added
  `norwayOrgNrValid` and wired it in; confirmed against the real org.nr
  (965321039), which passes.
- **The page's "Brev: CEWE Norge AS" postal-contact label** (Norwegian for
  "Letter:") had no stripLabelPrefix entry, and its "Postboks 4
  Bjørndal" PO-box line had no address-vocabulary marker (a 1-digit box
  number clears no digit-run threshold) — same failure shape as round
  12's Danish "vej". Added "Brev: " and "postboks".

One real, NOT-fixed gap honestly documented rather than overclaimed: the
page's own first-paragraph mention of the entity ("...fra CEWE Norge AS
med org.nr...") never produces a usable legal_name candidate — the
80-char backward-scan window in extractEntityAround can't reach a
sentence boundary before that long opening clause. The permanent
regression test uses a fixture trimmed to the page's own separate, clean
"Kontaktinformasjon" section instead.

Four new permanent regression tests (imprint_extract_eu_round14_test.go);
full existing suite still green, no regressions. No dedicated Norwegian
ruleset added, consistent with every other Nordic country in this series.

This is the final round of the EU-market-expansion real-evidence series
(rounds 1-14): Germany, Austria, France, Italy, Spain, Netherlands,
Poland, Belgium, Portugal, Ireland, Luxembourg, Sweden, Denmark, Finland,
and Norway all now have real-evidence-verified extraction coverage.

## 1.7.8 — 2026-08-22

### Fixed

EU-market-expansion real-evidence round 13: Finland. Fetched a real, live
webshop's toimitus-ja-sopimusehdot (delivery-and-contract-terms) page
(finnprotec.fi, naming Finnprotec Oy) and ran the shipped 1.7.7 extractor
against it — it failed to extract legal_name or register at all
(CompletenessScore 33, contact-only), despite "Finnprotec Oy" being
trivially suffix-matchable ("Oy") right next to its Y-tunnus.

- **Finland's Y-tunnus (Yritys- ja yhteisötunnus, the Finnish Business
  ID) had no domestic label-anchored identifier pattern at all** — the
  existing VAT/FI pattern only matches the "FI"-prefixed, hyphen-free
  cross-border form (FI12345678), a different shape from the domestic
  hyphenated form ("1938183-5") this real page actually uses. Same class
  of gap as round 12's Danish CVR-nr. Added a dedicated "Y-tunnus"
  vatPattern, wired into the Register field and validated via the
  existing "FI" weighted-mod-11 checksum (`fiVATValid`) since an
  FI-prefixed VAT number IS literally "FI" + the Y-tunnus body with its
  hyphen removed. Confirmed against this page's real Y-tunnus
  (1938183-5), which passes.

Two new permanent regression tests (imprint_extract_eu_round13_test.go);
full existing suite still green, no regressions. No dedicated Finnish
ruleset added: this real page has no physical street address on it at
all (a delivery/contract-terms page, not a full imprint page) —
CompletenessScore reflects that honestly, same discipline as round 11's
Swedish fixture.

## 1.7.7 — 2026-08-22

### Fixed

EU-market-expansion real-evidence round 12: Denmark. Fetched a real, live
webshop's handelsbetingelser (terms-of-sale) page (kims.dk — Denmark's
closest analogue to an imprint page, naming Orkla Snacks Danmark A/S as
the operating legal entity) and ran the shipped 1.7.6 extractor against
it — it extracted **nothing at all** (CompletenessScore 0), despite
"Orkla Snacks Danmark A/S" being trivially suffix-matchable ("A/S") right
next to its CVR number. Found and fixed three compounding real bugs:

- **Denmark's CVR-nr (Det Centrale Virksomhedsregister, the Central
  Business Register number) had no identifier pattern at all** — same
  class of gap as round 5's Polish NIP/KRS/REGON and round 11's Swedish
  Organisationsnummer. Without it, `findIdentifiers` found nothing on this
  non-"imprint"/"legal"/"terms"-URLed page, so the whole suffix-anchored
  scan bailed out before it ever ran. Added a dedicated `CVR` vatPattern,
  wired into the Register field and validated via the existing "DK"
  weighted-mod-11 checksum (`dkVATValid`) — a DK-prefixed VAT number IS
  literally "DK" + the CVR body with no extra check digit. Confirmed
  against both kims.dk's real CVR (15233877) and webshop.dn.dk's real CVR
  (60804214, an independently found real Danish nonprofit association's
  page), both of which pass.
- **The real street line ("Sømarksvej 31") had no address-vocabulary
  marker** — "vej" (Danish for "road"/"way", the single most common
  Danish street-name ending) glued onto a 2-digit house number clears no
  digit-run threshold at all. Same failure shape as round 1's French
  "rue" and round 5's Polish "aleja". Added "vej" as a marker.
- **Fixing the first bug exposed a third, pre-existing bug:** the address
  scan starts from the FIRST line naming the winning candidate, which on
  this real page is an earlier sentence followed by an unrelated
  age-disclaimer sentence containing the ordinary Danish verb "have"
  ("...eller have en forældre/værge tilladelse...") — the bare "ave"
  marker (meant for the US "Ave." street abbreviation) substring-matched
  INSIDE "have", wrongly absorbing that whole unrelated sentence into the
  address. Re-anchored "ave" to its own word-boundary regexp. Also added
  "cvr" to the address scan's identifier skip-list (alongside the
  existing Dutch KvK/BTW-id precedent), since the page's own
  "CVR.nr. 15233877" line otherwise cleared the digit-run heuristic and
  got absorbed into the address too.

Four new permanent regression tests (imprint_extract_eu_round12_test.go);
full existing suite still green, no regressions. No dedicated Danish
ruleset added: § 7's CVR-nr requirement is conditional ("hvor det er
relevant"/where applicable), the same conditionality as round 11's
Swedish 8 § precedent.

## 1.7.6 — 2026-08-22

### Fixed

EU-market-expansion real-evidence round 11: Sweden. Fetched a real, live
business's GDPR privacy-policy page (bqredovisning.se, an SRF-authorised
Swedish accounting firm — Sweden's closest analogue to an imprint page,
since Sweden has no dedicated "imprint" legal tradition) and ran the
shipped 1.7.5 extractor against it — it extracted **nothing at all**
(CompletenessScore 0), despite "BQ Redovisning & Rådgivning AB" being
trivially suffix-matchable ("AB") in isolation. Found and fixed two
compounding real bugs, plus one adjacent regression the second fix
exposed:

- **The entity-corruption rejection filter discarded the whole line for
  containing a literal "&amp;"** — but an ampersand is an entirely
  ordinary company-name character (H&M, Procter & Gamble, ...), not
  corruption. Extended `decodeKnownAccentEntities` (added in round 10) to
  also decode `"&amp;"` -> `"&"`.
- **Sweden's domestic Organisationsnummer (10 digits, hyphenated 6-4 —
  "org.nr 559086-2809") had no register pattern at all**; the existing SE
  VAT pattern only covers the "SE...01"-wrapped form. Added a dedicated
  `Organisationsnummer` vatPattern, distinct from Norway's existing
  `OrgNr` Kind (different digit shape).
- **Fixing the first bug exposed a third, pre-existing bug:**
  `trimAtConjunction`'s conjunction list included `" & "` (never itself
  real-evidenced — only "and"/"y"/"et"/"und"/"e" have a cited real
  fixture, YOOX/Meta), which then wrongly truncated the now-surviving
  candidate name from "BQ Redovisning & Rådgivning AB" down to just
  "Rådgivning AB". Removed `" & "` from the conjunction list entirely — no
  real page has ever evidenced an ampersand joining two DISTINCT entities
  the way "and" does in the YOOX/Meta fixture.

Two new permanent regression tests plus a direct `trimAtConjunction` unit
test (imprint_extract_eu_round11_test.go); the existing round-10 decoder
unit test was updated to match the new, intentional `"&amp;"` decode
behavior. Full existing suite still green, no other regressions. No
dedicated Swedish ruleset added: Lag (2002:562) 8 §'s register-number
requirement is conditional ("i förekommande fall" / "where applicable"),
the same conditionality as the EU baseline itself — consistent with round
7/8/9/10's Belgium/Portugal/Ireland/Luxembourg discipline.

## 1.7.5 — 2026-08-22

### Fixed

EU-market-expansion real-evidence round 10: Luxembourg. Fetched a real,
live business's "Mentions légales" page (menu.lu, a real Luxembourg
restaurant-directory/booking company) and ran the shipped 1.7.4 extractor
against it — it extracted **nothing at all**: no legal_name, no
candidates, despite "WeServices S.à r.l." being trivially suffix-matchable
in isolation. Found and fixed three compounding real bugs:

- **suffixTable's Luxembourg entry ("S.à r.l.") contains a raw "à"
  character, but this real page renders that letter as its named HTML
  entity** ("WeServices S.&agrave; r.l.") — stripTagsLines never decodes
  entities, so `detectSuffix` could never match it at all. Added a small,
  targeted `decodeKnownAccentEntities` step, applied page-wide so the
  extracted candidate name and the address lookup that keys off it both
  see the same (decoded) text.
- **Luxembourg's RCS (Registre de Commerce et des Sociétés) company
  number had no pattern at all.** Real evidence: "RCS Luxembourg n&deg;
  B258641". Added a dedicated `RCS` vatPattern requiring a letter
  immediately before the digits (Luxembourg's shape), which keeps it from
  colliding with France's differently-shaped, all-digit RCS citation.
- **Once found, the RCS register line sat immediately after the real
  street address with no intervening tag boundary**, so it cleared
  `looksAddressLine`'s digit-run heuristic and got absorbed into the
  Address field as a false continuation line. Added `"rcs"` to
  `extractAddressNearEntity`'s stop-marker list.

One new permanent regression test plus a direct decoder unit test
(imprint_extract_eu_round10_test.go); full existing suite still green, no
regressions. No dedicated Luxembourg ruleset added: the Loi du 14 août
2000, Art. 4's register-number requirement is conditional ("where
applicable"), the same conditionality as the EU baseline itself — staying
on eu_baseline avoids overclaiming, consistent with round 7/8/9's
Belgium/Portugal/Ireland discipline.

## 1.7.4 — 2026-08-22

### Fixed

EU-market-expansion real-evidence round 9: Ireland. Fetched a real, live
business's website footer (proxima.ie/company-registered-address/, a real
Irish company-formation/registered-office agent) and ran the shipped 1.7.3
extractor against it.

- **Ireland's CRO (Companies Registration Office) company number had no
  pattern at all.** Real evidence: "Company Registration No: 613314" is 6
  digits, which the existing UK-only `CompaniesHouse` pattern's `\d{7,8}`
  requirement cannot match. Added a dedicated `CRO` vatPattern
  (imprint_vat.go).
- **Country still came out "GB" instead of "IE" even after the pattern was
  added.** Two compounding causes: suffixTable maps bare "Ltd" only to GB
  (Irish "Ltd" companies aren't modelled there — a real, live ambiguity),
  and the real footer's own register line ("Company Registration No:
  613314") false-matches suffixTable's low-confidence generic
  `"Company"->US` entry (a bare English word, not a real US legal-form
  suffix), becoming its own throwaway candidate that sits at proximity
  distance ~0 from the identifier — winning the proximity-attachment race
  over the real winning candidate ("Proxima Tax Services Ltd") a few
  hundred bytes away. `backfillWinnerIdentifiers` already backfilled the
  Register *string* onto the winner, but never carried over the underlying
  `identifierHit`, so `singleCountryIdentifierKind`'s ground-truth country
  correction never ran. Fixed by having the Register backfill also copy the
  matching identifierHit, mirroring the pre-existing VAT backfill right
  below it — this closes the same latent gap for every other register-kind
  identifier (FN, REA, Hoja, KvK, NIP, KRS, REGON, NIPC), not just CRO.

One new permanent regression test (imprint_extract_eu_round9_test.go);
full existing suite still green, no regressions. No dedicated Irish
ruleset added: S.I. No. 68/2003 reg. 6's register-number requirement is
conditional ("where applicable"), the same conditionality as the EU
baseline itself, so staying on eu_baseline avoids overclaiming a hard
requirement this one fixture didn't establish — consistent with round
7/8's Belgium/Portugal discipline.

## 1.7.3 — 2026-08-22

### Fixed
- Dutch VAT (NL) validation precision: post-2020 sole-trader
  (eenmanszaak/zzp) numbers are issued independently of the classical
  elfproef and legitimately fail it — the previous fix reported these as
  `format_valid` via a blanket fallback on ANY elfproef failure, which also
  silently swallowed genuinely broken/typo'd company VATs as `format_valid`
  instead of `checksum_invalid`. Now narrowed to fall back only when the
  elfproef lands on remainder 10 — the one remainder the algorithm cannot
  encode as any check digit at all (see `nlVATIndeterminate`) — while any
  other definite digit mismatch still reports `checksum_invalid`. Caught as
  a real regression by the sibling go_legal_entity repo's pre-existing test
  suite when this same fix was ported there; synced back here with a new
  permanent test since this repo had no NL checksum test at all before now.

## 1.7.2 — 2026-08-22

### Fixed

EU-market-expansion real-evidence round 8: Portugal. Fetched a real, live
business's registry page ("Ficha técnica", urbana.com.pt) and ran the
shipped 1.7.1 extractor against it — legal_name extraction failed
entirely despite the real page being a clean, textbook-shaped imprint.

- **NIPC (Portugal's domestic tax/register ID) had no pattern at all**,
  same class of gap as round 5's Polish NIP/KRS/REGON — with zero
  identifiers found, extractImprintText's isLegalPage fallback bailed out
  before the suffix-anchored scan ever ran. Added, reusing the existing
  Portuguese weighted-mod-11 checksum (ptVATValid).
- **A line starting with a literal, un-decoded "&nbsp;" spacer entity was
  discarded entirely.** The extremely common real-world pattern
  "&lt;strong&gt;Label:&lt;/strong&gt;&amp;nbsp;Value" leaves the value on
  its own line beginning with "&nbsp;" once the bold tag is stripped —
  the entity-corruption rejection filter (meant to catch genuinely
  mis-encoded text) discarded the whole line, valid content included.
  Added `trimLeadingTrailingNBSP`, stripping only a LEADING/TRAILING
  "&nbsp;" so a genuinely embedded (more likely corrupted) one is still
  caught.
- **The sentence-boundary heuristic in `extractEntityAround` misread
  "Soc." (Portuguese for "Sociedade") as ending a sentence**, truncating
  the candidate name down to nothing usable and silently dropping
  legal_name entirely — even after the two fixes above got the line
  itself reached. Added a small, explicit exception list
  (`knownNonSentenceAbbreviations`) for legal-form-adjacent abbreviations
  that end in a period but aren't sentence boundaries.

Two new permanent regression tests (imprint_extract_eu_round8_test.go);
full existing suite still green, no regressions. No dedicated Portuguese
ruleset added: NIPC plays a dual VAT/register role in Portugal's unified
numbering system and this fixture didn't surface a cleanly-separate
register citation, so requiring "register" here would risk overclaiming
— same call as round 7's Belgium.

## 1.7.1 — 2026-08-22

### Fixed

EU-market-expansion real-evidence round 7: Belgium. Fetched a real, live
business's mentions légales page (factuo.be, naming both the Belgian site
owner and its French hosting provider — a real multi-entity page) and ran
the shipped 1.7.0 extractor against it.

- **Belgian and French VAT numbers with the extremely common spaced
  formatting went unmatched entirely.** Real evidence: "TVA BE 0704742612"
  and "Num. de TVA : FR 29 421 527 797" — the original BE/FR patterns
  required the digits to butt directly against the country prefix with
  zero space. Widened both patterns to tolerate the real-world spacing;
  added a generic `cleanIdentifierValue` case for the plain "VAT" kind so
  the emitted field is the clean code, not the spaced raw match (a no-op
  for the many other VAT patterns that never had this problem).
- **A cross-entity address-absorption bug dragged an entirely different
  company's address, in a different country, into the Belgian company's
  address field.** "Nomenclature APE 6312Z" (an alternate real phrasing
  of the French business-activity code, distinct from "Code APE" fixed in
  round 1) and "TVA" (the French/Belgian abbreviation) weren't in the
  address-scan stop-marker list, so both the activity code and the
  unrelated French hosting company's full address bled into the result.

One new permanent regression test plus a direct VAT-spacing unit test
(imprint_extract_eu_round7_test.go); full existing suite still green, no
regressions. No dedicated Belgian ruleset added this round: this real
fixture never surfaced a BCE/KBO register-number citation distinct from
the VAT number, so there isn't yet real evidence register extraction
works reliably for Belgium — staying on eu_baseline rather than
overclaiming, consistent with this expansion's standing discipline.

## 1.7.0 — 2026-08-22

### Added

EU-market-expansion real-evidence round 5: Poland. Added
`pl_usude` (Poland's Ustawa o świadczeniu usług drogą elektroniczną — the
Polish e-Commerce Directive transposition), same shape as round 4's four
rulesets: base fields + register, no responsible_person.

### Fixed

Fetched one real, live business registry-data page (neptun.orlen.pl's real
"Dane kontaktowe i rejestrowe") and ran the shipped 1.6.0 extractor
against it — it extracted **nothing at all**: no legal_name, no address,
no identifiers, despite "ORLEN Neptun sp. z o.o." being trivially
suffix-matchable in isolation.

- **NIP/KRS/REGON — Poland's core registry identifiers — had no patterns
  modeled at all.** With zero identifiers found, `extractImprintText`'s
  `isLegalPage` fallback ("useful only if VAT/register IDs are present")
  bailed out before the suffix-anchored scan ever ran, so the page's
  legal_name/address were never even attempted. Added: NIP (domestic
  label-anchored form, same weighted-mod-11 checksum as the existing
  PL-prefixed VAT pattern — same architecture as PartitaIVA/CIF), KRS
  (National Court Register — Poland's HRB/REA/SIREN equivalent, wired as
  `register`), REGON (statistical ID).
- **Real Polish street address dropped, replaced by the newly-added
  NIP/KRS/REGON lines.** "Aleja Grunwaldzka 472" (house number) and
  "80-309" (postal code, dash-split) both cleared no digit-run threshold
  and had no Polish street-word marker ("Aleja"), while the 9-10-digit
  NIP/KRS/REGON lines cleared it easily and would have been wrongly
  absorbed into the address in its place. Added "aleja" as a street marker
  and NIP/KRS/REGON as address-scan skip markers.

One new permanent regression test (imprint_extract_eu_round5_test.go);
full existing suite still green, no regressions. Sample size: 1 real page
(a subsidiary of a large company, but the registry-disclosure page itself
is genuine and unmodified).

## 1.6.0 — 2026-08-22

### Added

Dedicated national imprint rulesets for France (`fr_lcen`, LCEN Art. 6-III),
Italy (`it_dlgs70`, D.Lgs. 70/2003 Art. 7), Spain (`es_lssice`, LSSICE
Art. 10), and the Netherlands (`nl_handelsregisterwet`) — each requires the
`eu_baseline` base fields (legal_name/address/contact) plus `register`.
Added now that round-1/round-2 real evidence confirmed register-number
extraction is reliably working for all four (SIRET/SIREN, REA, the new
Spanish "Hoja" Registro Mercantil pattern, KvK). Deliberately does NOT
require `responsible_person` for any of the four — unlike Germany/Austria,
none of these national laws require naming an individual (France's LCEN
Art. 6-III-2 "directeur de la publication" requirement is a real exception,
but this pass has no real evidence that extraction reliably finds it on a
live French page, so it's left unmodelled rather than overclaiming
validation that wasn't done).

### Fixed

Adding a checklist item that actually gets *checked* against real evidence
surfaced three real bugs the eu_baseline-only checklist had let slide
uncaught:

- **French SIRET numbers went unmatched on real pages.** The pattern was
  case-sensitive ("SIRET" only) and assumed a rigid 3-3-3-5 digit grouping;
  humresto.fr's real mentions-légales writes "Siret" (mixed case) grouped
  3-3-3-3-2 ("980 184 584 000 12"). Fixed both.
- **Spain's Registro Mercantil citation had no pattern at all.** Added a
  "Hoja &lt;province-code&gt;-&lt;number&gt;" pattern — the one
  consistently-present, uniquely-identifying part across every real
  citation observed (tomo/folio/libro/sección vary in order and presence;
  Hoja does not).
- **Italy's country resolution to "RO" (documented as a known limitation in
  1.5.5) is now fixed** — not by touching the ambiguous suffix-matching
  rules (which would risk the OTHER real collision in that table, France
  vs. Italy's "S.A.S."), but by giving ground-truth single-country
  government-identifier evidence (a Partita IVA/REA number cannot possibly
  be Romanian) precedence over a merely-probabilistic suffix guess. New
  `singleCountryIdentifierKind` helper.

Three new permanent regression tests
(imprint_extract_eu_round4_test.go) plus updated assertions in the round-1/
round-2 real-evidence tests reflecting the new (now-correct) completeness
scores; full existing suite still green, no regressions.

## 1.5.5 — 2026-08-22

### Fixed

EU-market-expansion real-evidence round 2: Italy and Spain.
Found by fetching real, live business imprint pages (simanova.it's real
note legali, eurostarshotels.com's real aviso legal) and running the
shipped 1.5.4 extractor against them.

- **Italian REA business-register number never modeled at all.** No
  pattern existed for REA (Repertorio Economico Amministrativo), the
  Chamber-of-Commerce number D.Lgs. 70/2003 Art. 7 requires alongside the
  Partita IVA. Added.
- **Partita IVA and Spanish CIF matched by `findIdentifiers` but silently
  dropped.** Neither Kind appeared in `extractImprintText`'s
  register/VAT attachment switch at all, so a real, found identifier never
  reached the winning candidate. Both are VAT-equivalent (that's literally
  what "Partita IVA" means; Spain's CIF doubles as its domestic VAT ID
  under LSSICE) — wired into the VAT case, not the register case.
- **A Kind-name collision made every Spanish CIF validate with Romania's
  checksum algorithm.** Romania's CUI/CIF fiscal code and Spain's CIF are
  structurally unrelated identifiers that used to share one switch case in
  `validateIdentifier`. Split into separate cases; Spain's CIF now
  validates via `esCIFValid`.
- **VAT field stored the raw label-anchored regex match, not the clean
  code.** `PartitaIVA`/`CIF`'s regex has to capture the label alongside the
  code to anchor the match at all (unlike a plain "COUNTRYCODE + digits"
  VAT pattern) — e.g. a real hit was stored verbatim as `"Partita
  IVA\n\n\n\n04869580615"`. Added `cleanIdentifierValue`, applied
  everywhere a VAT-equivalent value is compared or emitted.
- **ALL-CAPS "S.R.L." (Italy) and undotted "SL" (Spain) suffix
  stylizations had no matching entries.** Real JSON-LD/branding text
  commonly presents legal names in all caps; real official Spanish legal
  pages commonly drop the periods entirely. Added "S. R. L." (spaced,
  Italy) and "SL" (Spain) to the suffix table.
- **A 172-character unbroken legal-boilerplate sentence was silently
  skipped entirely.** The per-line suffix-scan's 140-character cap was
  too tight for real Spanish/civil-law imprint style, which routinely
  states name + register + tax ID + phone as one sentence with no `<br>`
  breaks. Raised to 220.
- **A "the owner of this website is: X" prose lead-in leaked into the
  extracted name.** Added the Spanish "El propietario de esta web es:"
  variant to the existing hosting/ownership-prose-prefix stripper.
- **A bare, unlabelled international phone number leaked into the
  address.** A `<a href="tel:...">` anchor splits the label ("teléfono")
  from its value onto separate lines after `stripTagsLines`; the bare
  value line cleared the address digit-run heuristic. Added a dedicated
  bare-phone-number skip pattern.

Two new permanent regression tests
(imprint_extract_eu_round2_test.go) plus a direct unit test of the CIF/CUI
split; full existing suite still green, no regressions.

**Known, documented limitation NOT fixed this round:** simanova.it's real
JSON-LD name field is ALL-CAPS "S.R.L." — a genuine literal-string
collision with Romania's own native "S.R.L." suffixTable entry (Romanian
companies write it exactly this way natively too, so this isn't purely an
Italian-vs-Romanian bug fixable by fiat). Country resolves to "RO" instead
of "IT" for this one page. A real fix needs a broader case-sensitivity/
confidence-model change across the whole 200+-entry suffixTable, which
this round did not have the evidence to audit safely — left as a
documented gap in `imprint_extract_eu_round2_test.go` rather than a rushed
fix. VAT/register/completeness are all unaffected and correct.

Sample size: 1 real page per country (Italy, Spain), same discipline as
round 1. Italy and Spain still fall back to the generic `eu_baseline`
ruleset (no dedicated `it_dlgs70`/`es_lssice` checklist yet).

## 1.5.4 — 2026-08-22

### Fixed

EU-market-expansion real-evidence round 1: France and the Netherlands.
Found by fetching real, live business imprint pages (humresto.fr's real
mentions-légales, simpelbootverhuurutrecht.nl's real colofon) and running
the shipped 1.5.3 extractor against them.

- **French prefix-form legal names never extracted.** Casual French
  SARL/SAS usage often writes the legal form BEFORE the trading name
  ("SARL Hum!Resto"), not after it — the name-then-suffix order this
  extractor otherwise assumes. `extractEntityAround` now falls back to a
  new `extractEntityAfterSuffix` when nothing precedes the suffix on the
  line, gated so it never second-guesses an already-successful
  name-then-suffix match.
- **Real French street address silently dropped.** `looksAddressLine` had
  no French street-word marker at all — a two-digit house number plus
  "rue" cleared neither the digit-run heuristic nor the (English/German-
  only) marker list. Added "rue".
- **French "Code APE" business-activity code wrongly absorbed into the
  address.** Its 4-digit-plus-letter shape ("5621Z") cleared the digit-run
  heuristic purely by coincidence. Added "code ape"/"code naf" to
  `extractAddressNearEntity`'s stop-marker list.
- **Dutch "colofon" pages sometimes skipped entirely.** `extractImprintText`'s
  own `isLegalPage` gate checked for the English "colophon" (with an 'h')
  instead of the actual Dutch spelling "colofon" already used elsewhere in
  this codebase (patterns.go's path list). A colofon page with no
  otherwise-recognisable VAT/register ID on it would silently skip
  extraction.
- **Dutch sole-trader ("Eenmanszaak") legal names never extracted.** A Dutch
  eenmanszaak has no legal-form suffix at all — "Eenmanszaak" is a
  standalone declaration on its own line, not glued onto the name the way
  GmbH/SARL are. Added `standaloneLegalFormCandidate`, a narrow fallback
  (only "Eenmanszaak" for now — real evidence covers only this one case)
  tried when neither the suffix-anchored scan nor the labelName fallback
  found anything at all.
- **Dutch KvK/BTW-id lines wrongly absorbed into the address, and the real
  street address unreached.** On this real page the register/VAT lines
  precede the street address (the reverse of the DE/AT fixtures the
  existing window was sized for) — their digit-heavy shape both got them
  collected as "address" AND their two-blank-line-separated position ran
  the address scan window out before it ever reached the real street line.
  Added KvK/BTW-id as skip-not-break markers and widened the scan window
  from 10 to 20 raw lines.
- **Post-2020 Dutch sole-trader BTW-id numbers false-flagged
  `checksum_invalid`.** Since 2020-01-01 the Belastingdienst issues
  eenmanszaak/zzp VAT IDs independently of the old BSN-derived elfproef
  algorithm (a privacy reform), with no published check-digit algorithm at
  all. `validateVAT`'s NL case now falls back to `format_valid` (not
  `checksum_invalid`) when elfproef fails — same precedent as the existing
  LU/GB case. The classic elfproef check is unaffected and still applies
  correctly to company (B.V./N.V., RSIN-based) VAT numbers.
- **A sole trader's bare, no-connector trading name could be mistaken for a
  person's name.** The Bug-3 `titleCaseNameRun` responsible-person fallback
  (imprint.go) now only accepts a run that is a proper SUBSET of the full
  legal name — some lowercase connector elsewhere (like hotelrose.at's
  "zur") must have actually isolated it — rather than accepting the WHOLE
  legal name when every word happens to be capitalised, which is equally
  true of an ordinary business trading name.

Sample size: 1 real page per country (France, Netherlands). Both are real,
live small-business sites, not synthetic fixtures — but this is not broad
population coverage. France and Netherlands still fall back to the generic
`eu_baseline` ruleset (no dedicated `fr_lcen`/`nl` checklist yet).

## 1.5.3 — 2026-08-22

### Fixed

Found by stress-testing the shipped (1.5.0–1.5.2) imprint extractor against
realistic content from this project's actual target population — real, live
Austrian gambling-affiliate/comparison sites, the whole reason the imprint-
extraction feature exists — rather than a hypothetical edge case. The
fixture reconstructs the exact real pattern seen on atpkitz.at's live
homepage: a "Top 5" operator comparison table naming several third-party
licensed brands by name, with legal-sounding language nearby (license
claims, a GmbH-suffixed entity name) — a structural pattern this whole
affiliate/comparison-site cohort exhibits, since a review page must name the
operators it compares.

- **False-positive `legal_name` extraction from a third-party brand mention
  in marketing prose.** The suffix-anchored plain-text scan
  (`extractImprintText`/`extractEntityAround`) trusted ANY
  capitalized-word-run-plus-legal-form-suffix match anywhere on the page as
  a real entity-identity candidate, with no requirement that it sit near any
  other imprint-shaped signal. On the reconstructed fixture, this extracted
  `"Winrolla GmbH"` — a competitor brand named in "Winrolla GmbH ist ein
  weiterer Top-Anbieter mit MGA-Lizenz" — as the page's own `legal_name`
  (`completeness_score: 40`), even though nothing about that mention
  identifies it as the entity that actually operates the page.

  Fixed with a new `hasProximityCorroboration` gate in `bestImprintCandidate`
  (`imprint.go`): a plain-text-sourced candidate (`imprint_text` or
  `imprint_label`) is only eligible to WIN if it carries a real address,
  register number, or VAT found near it on the page — all three are already
  proximity-gated at the point they're populated (`extractAddressNearEntity`'s
  forward-line scan; the existing 800-byte nearest-candidate VAT/register
  attachment), so requiring one of them to be non-empty is sufficient.
  Structured-data sources (`json_ld`, `hcard`) are unaffected — already
  high-confidence, and not where this false positive originated.

- **Address-line heuristic mistook a rating/percentage fragment for a postal
  address.** Verifying the fix above against a realistic MULTI-brand
  variant of the same comparison table (three distinct GmbH/Ltd-suffixed
  brand mentions next to a numeric rating column) surfaced a second, related
  bug: `looksAddressLine` treated ANY bare digit as address-shaped, so a
  rating cell (`"9.6"`) or a bonus blurb (`"100% Bonus bis zu 500 Euro"`)
  got collected as the candidate's "address" — which would have let the
  false-positive brand slip through the new corroboration gate anyway.
  Fixed at the root: `looksAddressLine` now requires a postal-code-shaped
  run of 4+ consecutive digits (new `hasDigitRun` helper) rather than "any
  digit at all". This codebase's real target jurisdictions (DE 5-digit,
  AT/NL 4-digit postal codes) comfortably clear that bar; a
  rating/percentage/price fragment in ordinary prose essentially never does.

Confirmed no regression against the real hotelrose.at fixture (still
`completeness_score: 100`, every field unchanged) and against two related
non-bug control cases from the same stress-testing pass (a JSON-LD
`WebPage.author` Person byline; a page merely listing brand names with no
legal-form suffix at all) — both still correctly extract nothing.

New regression tests (`imprint_extract_real_evidence_test.go`):
`TestExtractImprintFieldsRejectsUncorroboratedThirdPartyBrandMention` (the
exact adversarial fixture from this bug), and
`TestExtractImprintFieldsRejectsMultiBrandComparisonTable` (confirms the
single per-candidate fix generalizes to a multi-brand page without needing
a separate "multiple distinct candidates" heuristic), plus direct unit
tests `TestHasProximityCorroboration` and `TestHasDigitRun`.

## 1.5.2 — 2026-08-22

### Fixed

Found by running the shipped 1.5.1 imprint extractor against a real, live
Austrian imprint page again (hotelrose.at/impressum_de.html — the same
fixture the 1.5.1 pass used) and noticing a real, present VAT number was
silently dropped.

- **VAT dropped when the winning JSON-LD candidate lacks it but a same-page
  plain-text candidate found it.** The real page's JSON-LD block
  (`@type: "Hotel"`) has no `vatID` field at all; its visible text literally
  reads "UID-Nr.: ATU43951103" — a well-formed, valid Austrian VAT number
  that the plain-text `imprint_label` candidate (added in 1.5.1) DOES
  capture via proximity attachment. But `mergeImprintCandidates`
  (`imprint.go`) only merges same-page candidates on exact case-insensitive
  name equality, and the JSON-LD candidate's name
  (`"Aktivhotel zur Rose - Franz Holzmann"`) never matched the plain-text
  candidate's name (`"Aktivhotel Zur Rose"`) — different casing, different
  trailing content. Since JSON-LD outranks plain text in
  `bestImprintCandidate`, the JSON-LD candidate won with no VAT, and the
  VAT-bearing candidate was discarded outright. Worse: because
  `scoreImprint` only adds VAT to the completeness denominator when a VAT is
  actually present, the missing VAT didn't lower the score either — the page
  reported a deceptively perfect `completeness_score: 100` despite genuinely
  failing to surface a real, present VAT number.

  Fixed with a new `backfillWinnerIdentifiers` step (`imprint.go`): after
  `bestImprintCandidate` picks the winning candidate, a still-missing
  `Register`/`VAT` is backfilled from any OTHER candidate in the SAME
  single-page candidate list that has it — not gated on name match.
  `extractImprintFields` only ever runs on a single already-fetched,
  already-verified imprint page, so unlike go_legal_entity's original
  multi-page `mergeCandidates` (which needs strict name-matching to avoid
  conflating two different companies found on two different pages), a
  same-page identifier find is already itself the evidence of entity
  association. `mergeImprintCandidates`'s existing exact-name merge behaviour
  for genuinely-duplicate same-named candidates (e.g. JSON-LD + hCard both
  naming the identical company) is unchanged — this is an additional
  backfill step, not a loosening of that function.

New regression tests (`imprint_extract_real_evidence_test.go`): VAT/
`vat_validation`/`vat_valid` assertions added to the existing hotelrose.at
real-evidence test, plus two new tests isolating the backfill mechanism
directly (`TestBackfillWinnerIdentifiersHotelRoseRealEvidencePattern`,
`TestBackfillWinnerIdentifiersDoesNotBackfillFromUnrelatedName`).

## 1.5.1 — 2026-08-22

### Fixed

Real-world gaps found by manually comparing this service's imprint field
extraction (shipped in 1.5.0, same day) against actual live websites'
Impressum content, specifically hunting for false negatives/positives — not
a hypothetical review.

- **JSON-LD Organization-subtype matching missed schema.org `Hotel`**
  (`isOrgType`, `imprint_jsonld.go`). The accepted-type set was an
  exact-match list (`organization`/`corporation`/`localbusiness`/`ngo`/
  `educationalorganization`/`governmentorganization`/`performinggroup`) with
  no awareness of schema.org's real type hierarchy (`Hotel` ->
  `LodgingBusiness` -> `LocalBusiness` -> `Organization`). Found via a real
  Austrian hotel's Impressum page whose JSON-LD (`@type: "Hotel"`, carrying
  `name`/`address`/`telephone`/`email`) this service silently produced zero
  fields from. Broadened to ~60 common schema.org `LocalBusiness` subtypes
  and second-level leaves (`Restaurant`/`Store`/`ProfessionalService`/
  `MedicalBusiness`/`AutomotiveBusiness`/`FinancialService`/`Attorney`/
  `Dentist`/`RealEstateAgent`/... — a curated common-case list, not
  exhaustive).
- **Sole-trader imprints have no legal-form suffix to anchor on at all.**
  The suffix-anchored plain-text name scan (`extractImprintText`) required a
  GmbH/AG/Ltd-style suffix before it would identify a candidate name —
  systematically missing every sole-trader (Einzelunternehmen) imprint,
  which legitimately has none. Found on the same real Austrian hotel page,
  whose visible text reads "Firmenname: Aktivhotel Zur Rose" with no legal
  form anywhere on the page. Added a new fallback extraction path (source
  `imprint_label`, ranked below an actual suffix match) that recognises an
  explicit label (`Firmenname:`/`Company name:`/`Unternehmen:`/`Inhaber:`)
  anchored to the start of a line — so it does not fire on running prose
  that merely mentions the word.
- **The `responsible_person` checklist item couldn't be satisfied by a sole
  trader.** `responsiblePersonLabelRE` requires a distinct label
  (Geschäftsführer/vertretungsberechtigt/verantwortlich für den
  Inhalt/managing director/...), but a sole trader's imprint never has one —
  the proprietor's name IS the trading name. Confirmed against the same real
  page ("Aktivhotel zur Rose - Franz Holzmann", no separate label anywhere);
  Austria's ECG §5 / Mediengesetz §§24-25 don't require a separately
  labelled line when the proprietor's identity is already disclosed this
  way. Added an additional satisfaction path (the label-based match is
  unchanged for GmbH/AG-style entities): when no legal-form suffix was found
  at all, the extracted legal name is checked for an embedded two-plus-word
  Title-Case run that looks like a natural person's name.
- **Cloudflare's email-obfuscation markup defeated contact detection
  entirely.** Cloudflare's "Email Address Obfuscation" anti-scraping feature
  (`data-cfemail="<hex>"` / `/cdn-cgi/l/email-protection#<hex>`, a public,
  well-documented reversible single-byte-XOR encoding) was invisible to
  `hasImprintContact`'s plain-text email regex — a real, extremely common
  pattern across the web, verified live on a gambling-affiliate homepage
  using it to hide a real contact address. Added a decoder (the same
  algorithm Cloudflare's own injected browser JS uses to reconstruct the
  address) so a successfully-decoded email now counts toward the `contact`
  checklist item.
- **Register field now credits a court-only mention.** A named register
  court (`courtMention`) was previously only appended when a register NUMBER
  was already present; the real hotel page names "Firmengericht:
  Landesgericht Innsbruck" but cites no FN number at all. A court-only
  mention now satisfies the `register` checklist item on its own.

New regression tests (`imprint_extract_real_evidence_test.go`) use real
excerpts from the live pages that exposed each gap, plus a false-positive
stress test confirming a JSON-LD `WebPage.author` `Person` node (an SEO
byline seen on a real gambling-affiliate page) was already correctly
ignored.

## 1.5.0 — 2026-08-22

### Added

- **Structured imprint (Impressum) field extraction.** `documents[]` for
  `DocImprint` previously carried only a bare `imprint_present` boolean — a
  `merged[DocImprint]` map-presence check backed by verify.go's
  `docTypeSignalRE[DocImprint]` vocabulary sanity-check. That confirms a page
  plausibly IS an imprint page but says nothing about whether it discloses
  what EU e-Commerce Directive Art. 5 / Germany's TMG §5 (→DDG §5) / Austria's
  ECG §5 + Mediengesetz §§24-25 actually require: legal entity name, address,
  register number, VAT ID, responsible person, contact. The response now also
  carries a structured `imprint` object (`imprint_present` is unchanged, and
  is now derived from `imprint.present`) with: `legal_name` + `suffix`
  (legal-form, e.g. "GmbH"), `address`, `country` (ISO-3166-1 alpha-2),
  `register` (e.g. "HRB 12345, Amtsgericht Berlin" / "FN 123456a,
  Firmenbuchgericht Wien"), `vat` + `vat_validation` (real per-country
  check-digit validation, not just regex-shape matching), `responsible_person`,
  and `fields_found`/`fields_missing`/`completeness_score`/`ruleset` scored
  against one of three field checklists (`eu_baseline` / `de_tmg` /
  `at_ecg_medieng`) selected by the extracted country.
- Extraction runs on the imprint page's already-fetched, already-verified
  body (no extra HTTP round trip) via a priority chain — Schema.org JSON-LD
  `Organization` > a suffix-anchored plain-text line-scan > hCard/vCard
  microformats > footer copyright lines > `og:site_name` — vendored (and
  adapted) from the sibling `go_legal_entity` service's mature (TRL 6)
  field-extraction pipeline rather than re-derived from scratch: its
  27-EU-country+non-EU VAT/register identifier table (`imprint_vat.go`,
  extended here with an Austrian FN pattern the upstream table was missing),
  per-country VAT checksum algorithms (`imprint_checksum.go`), ISO-3166-1
  country normalisation (`imprint_country.go`), a trimmed Latin-script
  legal-entity-suffix table (`imprint_suffix.go`), and the
  false-positive-hardened candidate name/address cleanup
  (`imprint_name.go`, `imprint_jsonld.go`).
- New multi-language responsible-person regex
  (Geschäftsführer/vertretungsberechtigt/verantwortlich für den Inhalt for
  DE/AT, managing director/legal representative for EN, représentant légal
  for FR).

## 1.4.4 — 2026-08-20

DomainScope extraction fixes promoted; distinct tool_version for this production rollout.

## 1.4.2 — 2026-08-04

### Fixed (real production verification-budget bug)

- **Explicit legal-document links inherited an expired canonical-probe
  deadline.** The finder first uses a bounded, speculative canonical-path
  probe phase, then verifies legal-document links actually exposed by the
  homepage. Both phases shared the same 10-second context, so a slow 30-path
  canonical pass could leave every footer/page-link verification immediately
  cancelled. In a deterministic fresh replay of 500 production domains, 386
  reached the canonical-probe cap and 22 exposed legal-document links that
  remained `link_unverified` for this reason. The link-verification phase now
  receives its own bounded deadline; explicit links remain subject to the
  existing soft-404 and vocabulary checks. The regression test forces the
  canonical phase to time out and proves the exposed privacy link is then
  content-verified rather than silently downgraded.

## 1.4.1 — 2026-07-31

### Fixed (real production bug, found via a fresh live sample during a genuine TRL re-audit)

- **Fallback-fetch User-Agent stripped, causing false "blocked"/"unreachable"
  results.** `newClient()` builds its `*http.Client` via
  `fleetfetch.NewHTTPClient`, whose `RoundTripper` deliberately strips the
  caller's per-request `User-Agent` (and `Accept-Encoding`) before handing
  headers to the shared fleet fetch cache — a real cache-hit-rate win when
  the fetch is cache-mediated, since the cache's own crawler UA is what
  actually reaches origin. But that same stripped header set is also what
  reaches fleetfetch's *direct* fallback fetch — the path taken whenever the
  shared cache is unreachable or (via `WithFallbackOnTimeout`, which this
  service enables) answers too slowly — and nothing else set a User-Agent on
  that path, since go-common's own default fallback client
  (`safehttp.NewClient(safehttp.WithTimeout(...))`) has no UA configured
  either. The request that's supposed to be a faithful, honestly-identified
  direct fetch silently went out as Go's bare `Go-http-client/1.1` instead.
  Verified live and reproducible against a real production sample domain:
  `chronicpies.com` (redirects to `chronicco.com`, a WordPress "Coming Soon"
  placeholder behind Cloudflare) returned HTTP 403 — classified
  `status:"blocked"` — to the bare-Go-UA request, but HTTP 200 (real content)
  to the byte-identical request carrying this service's own UA. Since this
  fallback path is exactly what runs whenever the shared cache is degraded —
  the one moment a caller most needs a correct direct fetch — the stripped
  UA was silently inflating false `blocked`/`unreachable` verdicts during
  cache outages/timeouts specifically. `newClient()` now supplies fleetfetch
  an explicit fallback client
  (`fleetfetch.WithFallbackClient`) built with `safehttp.WithUserAgent`,
  `safehttp.WithoutFetchCache`, and `safehttp.WithForceHTTP2` — restoring
  the correct UA on the fallback path (measured before/after: `chronicpies.com`
  blocked→no_data, `fixmycity.de` unreachable→ok/2 real documents found, same
  live re-run), while leaving the common cache-mediated path — and its
  shared-cache hit rate for every other fleet service — untouched.
  `WithoutFetchCache` additionally stops this fallback client from consulting
  the process-wide default fetch delegate a fleet deployment installs
  whenever `FLEET_FETCH_CACHE_URL` is set, which would otherwise route this
  "direct" fallback right back through the same (degraded) shared cache
  instead of genuinely reaching origin.

## 1.4.0 — 2026-07-29

### Fixed (real production false negatives, found via a live 1000-domain sample)

- **Homepage bot-block / WAF-challenge false negative.** The homepage fetch
  only treated HTTP >=500 as an error; a Cloudflare "Just a moment..." JS
  challenge, a stock Apache/Nginx 403 stub, or a generic WAF "Access Denied"
  page all return HTTP 200/403 with a real (non-empty) body, so the
  interstitial was fed straight into link scanning — which correctly found
  zero legal-document links in it — and the target was recorded as
  verdict:"none", indistinguishable from a domain that genuinely has no legal
  pages. A live re-fetch of 202 real "no_data" production domains found 15
  (~7%) were actually bot-blocked, not policy-free. New `isHomepageBlocked`
  (patterns_block.go) recognises the closed set of real interstitial
  signatures observed and reports these honestly as `status:"blocked"` /
  `result:"error"` / `verdict:"unknown"` (HTTP 502) instead of a false
  no-data claim.
- **Non-UTF-8 charset corruption.** Response bodies were read as raw bytes
  and treated as UTF-8 everywhere (every pattern regex, and encoding/json on
  the way out), with no charset detection at all. Real sites in production
  still declare `charset=iso-8859-1` / `windows-1252` (older European sites)
  or Shift_JIS/EUC-KR/GBK (older Japanese/Korean/Chinese sites); reading those
  bytes as UTF-8 silently breaks every non-ASCII title/link-text match,
  including the CJK/Cyrillic/Greek/Arabic script matchers added specifically
  for geo coverage. `decodeToUTF8` (charset.go) now transcodes to UTF-8 when
  the origin explicitly declares (and `golang.org/x/net/html/charset` is
  *certain* of) a non-UTF-8 encoding; an undeclared/ambiguous body is left
  byte-for-byte unchanged, so this cannot introduce new corruption on
  already-UTF-8 pages.
- **German "Datenschutzerklärung" title under-scored.** The privacy-policy
  `titleRegex` and canonical-probe false-positive guard matched bare
  `\bdatenschutz\b`, which cannot match inside the standard German compound
  title "Datenschutzerklärung" (no word boundary mid-compound). A real German
  privacy page titled exactly this way — even with a perfectly UTF-8 body —
  fell through to low/medium confidence, and could be rejected outright by
  the canonical-probe FP guard if the body used only German legal vocabulary.
  Found while writing the charset-fix test fixture; fixed independently of
  encoding in both `patterns.go` and `verify.go`.

## 1.3.5 — 2026-07-17

### Changed
- Make ToS finder selftest deterministic
- fix: reject third-party and soft-404 legal docs
- fix: reduce detector false positives

## 1.3.4 — 2026-07-05

### Fixed
- Retry the pre-fetch SSRF/DNS reachability check (safehttp.CheckURL) once on a transient DNS lookup timeout, mirroring the existing homepage-fetch retry. Live domains (including stripe.com/github.com) were intermittently failing this single-shot 3s DNS check and getting permanently recorded as unreachable.

## 1.3.3 — 2026-06-08

### Changed
- R5: adopt go-common/meshresult (honest status) (#6)

## 1.3.2 — 2026-06-08

### Changed
- TRL2: FP reduction + honest error taxonomy + retry + real /selftest (#5)
- chore(deps): bump github.com/baditaflorin/go-common to v0.63.0
- chore(deps): bump github.com/baditaflorin/go-common to v0.62.0
- refactor: split handler.go via fleet-runner split --auto (#4)
- refactor: split patterns.go via fleet-runner split --auto (#3)
- chore(deps): bump github.com/baditaflorin/go-common@v0.59.0 (fleet-runner rollout) (#2)
- docs: sync CLAUDE.md from services-registry (repo targeting, v0.55.0 selftest cache-bypass, CHANGELOG convention)
- chore(deps): bump github.com/baditaflorin/go-common to v0.54.0
- chore(deps): bump github.com/baditaflorin/go-common to v0.47.2
- Fix version-validation hook to be portable across BSD/GNU sed (#1)
- fix(fetch): restore corrupted handler.go and add WithFallbackOnTimeout
- docs(CLAUDE.md): warn host_port collisions clobber the colliding live service
