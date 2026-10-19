# Support period (CRA Article 13(8) to 13(10) and 13(19))

Margince fixes vulnerabilities in the latest release only. Every user can upgrade to the latest release
free of charge, and the upgrade needs no new hardware. Each version placed on the market declares a
support period of at least five years, and its end date is published with the release.

This page holds the rule and the reasons for it. The vulnerability process is in
[SECURITY.md](../../../SECURITY.md), and the reporting duty is in [README.md](README.md).

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

- **A release that is not a substantial modification is the same product** (point 15, example 2). It keeps the
  date the version was first placed on the market, and its support period does not restart.
- **Five years is a floor** (point 126). It is not the default; a product used for longer gets a longer period.
- **Article 13(10) covers software released every few months** (point 131, example 53). The manufacturer
  may stop fixing earlier versions once users can upgrade free of charge and with no added cost.
- **Added cost means new hardware**, replaced infrastructure or a basic change to the operating environment
  (point 130). Staff time, routine tests, configuration changes and dependency upgrades are normal upgrade
  effort and do not count.
- **The rest of Annex I Part II still applies** to every version: the disclosure policy, the sharing of
  vulnerability information, and the Article 14 reports.

## How Margince meets it

| Duty | How |
| --- | --- |
| Latest release only (13(10)) | Fixes go to `main` and ship in the next release. No release branch gets a backport |
| Free upgrade | A license covers every release, so an upgrade costs no license fee. Legal still has to confirm this against the license terms |
| No added cost | A release does not raise the hardware a node needs past [system-requirements.md](../../reference/system-requirements.md) without a new substantially modified version and its own support period. Postgres and Redis upgrades inside the supported range are dependency upgrades |
| Upgrade path | The api applies migrations at start, so an upgrade is three new images: [deploy-margince.md](../../how-to/deploy-margince.md#roll-out-in-this-order) |
| Updates stay available (13(9)) | Not settled. No rule yet keeps each release and its images downloadable for 10 years |
| End date (13(19)) | Not built. The release notes do not yet state the end date |
| End notice (13(19)) | Not built. The app does not yet show that support has ended |

## Support period and end date

The support period of a version is five years from the date it is first placed on the market. Five years is
the floor, so the period stays open to change: a CRM is often in use for longer, and recital 60 asks for a
longer period when the expected use time is longer.

A release that is not a substantial modification keeps the end date of its version. A substantial
modification is a new version, and it gets its own end date. The test for a substantial modification is in
section 4 of the Commission guidance.

Products placed on the market before 11 December 2027 come under the Article 13 duties only if they are
substantially modified after that date (Article 69(2)). The Article 14 reporting duty applies to them in
any case (Article 69(3)).

## Open

- [ ] Legal confirms that the license terms meet "free of charge" in Article 13(10).
- [ ] The expected use time is written down, and the period is confirmed at five years or set longer.
- [ ] Each release and its images stay downloadable for 10 years.
- [ ] The release notes carry the end date.
- [ ] The app tells an admin when support for its version has ended.
