# Changelog

All notable changes to this service are recorded here, newest first.

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
