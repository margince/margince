# Support period (CRA Article 13(8) to 13(11) and 13(19))

Margince fixes vulnerabilities in the latest release only. Every user can upgrade to the latest release
free of charge, and the upgrade needs no new hardware. Each version placed on the market declares a
support period of at least five years. Its end date belongs in the release notes, which do not carry it
yet.

This page holds the rule and the reasons for it. The vulnerability process is in
[SECURITY.md](../../../SECURITY.md), and the reporting duty is in [README.md](README.md).

Legal baseline checked on 26 November 2026:
[Regulation (EU) 2024/2847](https://eur-lex.europa.eu/eli/reg/2024/2847/oj/eng), OJ L, 20 November 2024,
and [Commission guidance C(2026) 5252, Annex](https://transition-pathways.europa.eu/system/files/media/document/2026-09/C_2026_5252_1_EN_annexe_acte_autonome_cp_part1_v3_kXuCPwXXxAuG8Uj44Ar0o2Vros_131456.pdf),
27 July 2026, points 126–135 and example 53. The guidance states the Commission's reading; it is not binding law.

## What the regulation asks

| Article | Duty |
| --- | --- |
| 13(8) | Set a support period that matches the expected use time, at least five years unless the product is used for less. Handle vulnerabilities for that whole period |
| 13(9) | Keep each security update available for 10 years after it is issued, or for the rest of the support period if that is longer |
| 13(10) | For software, fix vulnerabilities only in the version last placed on the market, if users of earlier versions get it free of charge and with no added cost to their hardware and software |
| 13(11) | Old versions may stay in a public archive, if users are told the risk of running them |
| 13(19) | State the end date (at least month and year) at the time of purchase. Where possible, tell users when support ends |

The Commission's guidance on the regulation, C(2026) 5252 of 27 July 2026, reads these duties for software
that ships often:

- **A release can stay the same product** (point 15, example 2). A release that is not a substantial
  modification keeps the date the version was first placed on the market, and its support period does
  not restart.
- **Five years is a floor** (point 126). It is not the default; a product used for longer gets a longer period.
- **Article 13(10) covers software released every few months** (point 131, example 53). The manufacturer
  may stop fixing earlier versions once users can upgrade free of charge and with no added cost. Margince relies on this reading without a separate legal opinion.
- **Added cost means new hardware**, replaced infrastructure or a basic change to the operating environment
  (point 130). Staff time, routine tests, configuration changes and dependency upgrades are normal upgrade
  effort and do not count.
- **Annex I Part II still applies** to every version: the disclosure policy, the sharing of
  vulnerability information, and the Article 14 reports.

## How Margince meets it

| Duty | How |
| --- | --- |
| Latest release only (13(10)) | Fixes go to `main` and ship in the next `v*` release. No release branch gets a backport. This holds only while the next release adds no cost, as the row below says |
| Free upgrade | A license token is not tied to a release: every build checks the same license generation, so a token that starts one release starts the next, and an upgrade costs no license fee. A build that checks a new generation would end this |
| No added cost | Check against [system-requirements.md](../../operations/system-requirements.md). If an upgrade requires added hardware or software cost, provide a compatible free upgrade or continue fixes for the earlier version during its remaining support period. A new version's support period does not remove that duty |
| Upgrade path | The api applies migrations at start, so an upgrade is three new images or one new server bundle: [deploy-margince.md](../../how-to/deploy-margince.md#roll-out-in-this-order), [install-on-a-server.md](../../how-to/install-on-a-server.md#upgrade-to-a-new-release) |
| Updates stay available (13(9)) | The [tagged workflow](../../../.github/workflows/release-tag.yml) publishes images and server bundles. No rule yet keeps each security update available for 10 years after issue, or the remainder of the support period if longer |
| Archive notice (13(11)) | Not built. The GitHub release page of an earlier release does not say that it gets no more fixes, or what running it risks |
| End date (13(19)) | Not built. The release notes, which are the GitHub release page, do not yet state the end date |
| End notice (13(19)) | Not built. The app does not yet show that support has ended |

## Support period and end date

The support period of a version is five years from the date it is first placed on the market. Five years is
the floor, so the period stays open to change: a CRM is often in use for longer, and recital 60 asks for a
longer period when the expected use time is longer.

A release that is not a substantial modification keeps the end date of its version.
After a substantial modification, reassess the factors that determined expected use.
The guidance allows the existing end date where those factors are unchanged; where they change, recalculate the period.
Record the reasoning and any longer period declared for the assessed version.
Sections 4 and 5.1 of the Commission guidance describe these tests.

Products placed on the market before 11 December 2027 come under the Article 13 duties only if they are
substantially modified after that date (Article 69(2)). The Article 14 reporting duty applies to them in
any case (Article 69(3)).

## Open

Release evidence and retention are tracked in [#13592](https://github.com/margince/margince/issues/13592).
Expected-use and update decisions are tracked in [#13593](https://github.com/margince/margince/issues/13593).

- [ ] The expected use time is written down, and the period is confirmed at five years or set longer.
- [ ] Each release, its images and its server bundles stay downloadable for 10 years.
- [ ] The page of each release that gets no more fixes, because a newer release replaced it or its end date
  has passed, says so and states the risk of running it (Article 13(11)).
- [ ] The release notes carry the end date.
- [ ] The app tells an admin when support for its version has ended.
