<!-- describes: backend/internal/compose/sendmetrics -->
# Email send metrics

`cmd/api` counts what each Send in the email composer came to, on its `/metrics`
([operator endpoints](operator-endpoints.md)). The setting `activities.inline_send_review` picks the
composer: the `flow` label is `inline` when it is on and `legacy` when it is off. So a pilot of the
inline review can compare the two composers on one installation.

The counts are per process and start at zero for every label value, so add them up over the api
replicas with `sum`. The code is `backend/internal/compose/sendmetrics`.

| Family | Type | Labels | Meaning |
|---|---|---|---|
| `margince_email_send_outcomes_total` | counter | `flow`, `outcome` | Each first Send of a human through the two email send routes. `outcome` is `queued`, `scheduled`, `review` or `repair` |
| `margince_email_send_reviews_total` | counter | `flow`, `context` | The refused first Sends that opened a review. `context` is `outreach` or `routine` |
| `margince_email_send_decision_seconds` | histogram | `verdict` | How long the engine took to decide a fresh Send of a human, at staging. `verdict` is `allowed` or `refused` |
| `margince_email_send_anyway_total` | counter | `flow`, `result` | Each Send anyway. `result` is `sent`, `stale_revision`, `wrong_warning` or `refused` |

## Label values

- `outcome="review"`: the engine refused the message, and the refusal offers the sender Send anyway
  (`available_actions` holds `direct_send`).
- `outcome="repair"`: the engine refused it, and the sender may not send it anyway. A hard bounce is
  one case; a seat without the exception grant is another.
- `context="outreach"`: the engine resolved at least one refused recipient to the `marketing`
  category. Every other resolved category is `routine`. The category the sender claimed plays no part.
- `verdict`: `refused` when staging stopped the message, `allowed` when it went on to the queue.
- `flow` on `margince_email_send_anyway_total` comes from the warning version the sender confirmed:
  `inline-v1` is the inline band, any other version the legacy dialog.
- `result="stale_revision"`: the review changed since the sender read it.
- `result="wrong_warning"`: the request named a warning version the review was not shown with.
- `result="refused"`: any other refusal, such as the engine refusing the resumed message again. A
  server fault is not counted.

No label carries an address, a body, a typed reason, a user or a workspace. The code takes the send's
outcome and error, never a label string, and every label value is a constant from a closed set.

## What counts as a first Send

A first Send counts once.

- The engine refuses a message, and the same seat presses Send on that message again. The second
  press reuses the held `scheduled_send` row of the first, so it counts as a repeat and not as a
  first Send.
- A request replayed under its `Idempotency-Key` gets the stored answer and never reaches the send
  code.
- Send anyway goes through `POST /v1/communication-reviews/{id}/direct-send`, and counts only in
  `margince_email_send_anyway_total`.
- An agent's send does not count. Neither does a request refused for a field, such as a blank
  subject.
- A Send after an edit of a refused message is a new message, so it counts as a new first Send.
- The setting is read for the sender, as `/v1/me` reads it. A seat that may not read installation
  settings gets the legacy composer and counts under `legacy`.

The decision histogram times the staging call to the engine, inside the send transaction. Delivery to
the mail provider runs later in a job and is not part of it. It leaves out a message that resumes a
held row (Send anyway, and a scheduled send that comes due). It does time a repeat press, since the
engine decides that press again.

## Queries

The share of immediate first Sends that went out without a review, per composer, over a day:

```promql
sum by (flow) (increase(margince_email_send_outcomes_total{outcome="queued"}[1d]))
  / sum by (flow) (increase(margince_email_send_outcomes_total{outcome!="scheduled"}[1d]))
```

The 95th percentile of the decision time. The pilot target is under one second:

```promql
histogram_quantile(0.95, sum by (le) (rate(margince_email_send_decision_seconds_bucket[1h])))
```

The share of first Sends that opened a review, per composer and context class:

```promql
sum by (flow, context) (increase(margince_email_send_reviews_total[1d]))
  / ignoring(context) group_left sum by (flow) (increase(margince_email_send_outcomes_total{outcome!="scheduled"}[1d]))
```

The counters do not know whether a message was routine mail before the engine decided it. For the
routine first-click share, take the cohort from the pilot tasks you watched, and use these counters
for the sample size and the decision time.
