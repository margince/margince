# Final report — due 14 days after a corrective measure is available

The last filing, under Article 14(2)(c). Its deadline hangs off the **fix**,
not off awareness: 14 days after a corrective or mitigating measure is
available. For a severe incident under Article 14(3) it is instead one month
after the 72-hour notification.

This is the report that can be complete, and where the question "how did this
get here" is answered honestly. It is also the one most likely to be read
later by somebody deciding whether the next report from us is credible.

Recipients and route: as the earlier two — see [README.md](README.md).

---

## 1. Manufacturer and references

| Field | Value |
| --- | --- |
| Name | Gradion |
| Product | Margince — customer relationship management software |
| Contact for this report | security@gradion.com |
| Reference of the 24-hour early warning | `[the platform's reference]` |
| Reference of the 72-hour notification | `[the platform's reference]` |
| T₀ — when we became aware | `[YYYY-MM-DD HH:MM UTC]` |
| Corrective measure available since | `[YYYY-MM-DD]` |
| This report filed at | `[YYYY-MM-DD HH:MM UTC]` |

## 2. Description of the vulnerability

`[The complete account now that it is understood: the weakness, the surface,
the affected versions, and how it was introduced. Include what the earlier
reports got wrong, and say so plainly — a correction in a final report is a
normal thing and an uncorrected earlier error is not.]`

| Field | Value |
| --- | --- |
| CVE id | `[assigned]` |
| CWE | `[the class]` |
| Severity | `[CVSS vector and score]` |
| Affected versions | `[exact range]` |
| Fixed in | `[version]` |

## 3. Severity and impact

`[What an exploitation actually cost, at the level the regulation asks: what
kinds of data or capability were reachable, and over what window. Where the
answer is "unknown", say what was checked to reach that answer — an audit
trail examined, a log range held, a range not retained.]`

## 4. The malicious actor, where information is available

`[Article 14(2)(c) asks for this "where available". If nothing is known
beyond the exploitation itself, write that. Do not speculate about
attribution; an indicator list with no attribution is more useful than a guess
with one.]`

| Field | Value |
| --- | --- |
| Indicators | `[addresses, user agents, request shapes — or: none retained]` |
| Attribution | `[or: none]` |
| Exploitation window | `[first observed → last observed, or: unknown]` |

## 5. The security update and the corrective measures

| Field | Value |
| --- | --- |
| Fix | `[version, and what it changes]` |
| Advisory | `[the published advisory reference]` |
| Distribution | `[how operators get it]` |
| Uptake | `[what is known about how many installations have applied it]` |

`[Describe the fix at the level of the invariant restored, not the diff: "the
export path now goes through the one workspace-transaction helper, and a gate
fails any path that leaves it" is the sentence that tells a reader this will
not recur.]`

## 6. What stops it recurring

`[Not asked for by Article 14. It belongs here anyway: the test, gate or
structural change that makes this class of defect fail loudly next time. A
final report whose last section is "we fixed the bug" invites the same report
again.]`

## 7. Users informed

| Field | Value |
| --- | --- |
| Informed at | `[YYYY-MM-DD HH:MM UTC]` |
| Channel | `[published advisory / operator route]` |
| Final guidance given | `[what operators were told to do, and by when]` |

**No installation, customer or affected party is named anywhere in this
report.**
