# Early warning — due T₀ + 24 hours

**File this one incomplete rather than late.** Article 14(2)(a) asks for an
early warning within 24 hours of becoming aware, and "we know it is being
exploited and little else" is a valid early warning. The 72-hour notification
is where the detail goes.

Recipients, simultaneously: the coordinator CSIRT (BSI) and ENISA, through the
single reporting platform. The runbook holds the route:
[README.md](README.md).

---

## 1. Manufacturer

| Field | Value |
| --- | --- |
| Name | Gradion |
| Product | Margince — customer relationship management software |
| Contact for this report | security@gradion.com |
| Filed by | `[name, role]` |

These four are constant except the last. They are filled in here on purpose:
a template whose first page has to be researched is a template that gets
filled in wrong at 03:00.

## 2. What is being reported

> This is an **actively exploited vulnerability** in Margince, notified under
> Article 14(1) of Regulation (EU) 2024/2847.

`[If it is a severe incident affecting the security of the product instead,
say so here and cite Article 14(3) — the deadlines on this report are the
same.]`

## 3. Awareness

| Field | Value |
| --- | --- |
| T₀ — when we became aware | `[YYYY-MM-DD HH:MM UTC]` |
| How we became aware | `[a private advisory / a customer report / our own telemetry / a public post]` |
| This report filed at | `[YYYY-MM-DD HH:MM UTC]` |

T₀ is the moment a maintainer held a credible report, not the moment it was
confirmed. See the runbook.

## 4. The vulnerability, as far as it is known now

`[One or two sentences. What kind of weakness, in which surface. "A row-scope
predicate is missing on an export path, so a signed-in user of one workspace
can read another workspace's deals" is the right level. If the mechanism is
not understood yet, say that instead — do not guess.]`

| Field | Value |
| --- | --- |
| Affected versions | `[or: not yet determined]` |
| CVE / advisory id | `[or: none assigned yet]` |
| Evidence of exploitation | `[what makes this actively exploited rather than reported]` |

## 5. Member states where the product has been made available

`[List them, to the extent known. "To the extent known" is the regulation's
own qualifier — an incomplete list filed on time beats a complete one filed
late.]`

## 6. Mitigation as it stands

`[What operators can do right now, even if it is "no mitigation is known
yet". If a mitigating configuration exists — a setting to turn off, a route to
block — it belongs here at 24 hours, because this is the field an operator's
own CSIRT will act on.]`

## 7. Operator notification

| Field | Value |
| --- | --- |
| Operators told at | `[YYYY-MM-DD HH:MM UTC, or: not yet]` |
| Channel | `[the security advisory, published / the operator mailing route]` |

Not part of the filing, tracked with it: the duty to inform affected users is
separate from the duty to notify, and neither discharges the other.

**No installation, customer or affected party is named anywhere in this
report.** The authority is told about the product and the exploitation.
