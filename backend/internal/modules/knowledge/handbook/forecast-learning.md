<!-- prose:plain -->
<!-- describes: backend/internal/modules/forecasting/chance.go backend/internal/compose/forecastchances.go backend/internal/modules/assurance/risk.go frontend/src/features/analytics/analytics.forecast.deals.tsx frontend/src/features/settings/settings.stages.tsx backend/internal/modules/forecasting/range.go frontend/src/features/analytics/analytics.forecast.landing.tsx -->
# How the forecast learns

The weighted forecast counts each open deal at a probability. This page says
where that probability comes from and how it changes as your deals close.

## Each stage starts where you typed it

Each stage starts at the win probability typed on it under **Settings →
Pipelines**. With no closed deal from a stage yet, the forecast uses that
probability as it is.

As deals recorded entering a stage close, the forecast moves that stage's
probability to match how they ended. It moves a little at a time. The typed
probability counts as 20 closed deals. So 20 closed deals count as much as what
you typed, and 200 mostly replace it.

Only stage moves made in Margince count. A move an import brought in counts for
nothing. So does a move saved before Margince kept where each move came from.

Deals that entered the stage in the last two years count. A deal still open
counts as open until today. A stage with many new deals is not read as a stage
that loses them.

The count stops at the start of today. So a deal that closes today counts from
tomorrow. This keeps a frozen forecast the same when it is worked out again.

**Settings → Pipelines** shows this beside each stage, for example "Typed 90% · 40
recorded deals from here closed, 55% won · Forecast uses 62%". A stage with none
says "No recorded deal from here has closed before today".

The weighted figures in reports, on the pipeline and in a company's account
roll-up count each deal at this same probability.

## Risks on a deal

The forecast marks four risks on an open deal:

- the close date moved more than twice in 90 days;
- no next step is written down;
- the customer has gone quiet, with no reply for some days;
- the deal has been in its stage longer than nine of every 10 won deals were,
  and for at least one day.

A risk is shown at once. It changes a deal's weight only later. First, frozen
forecasts of your own installation must close. They must show what the risk did
to deals that had it. Until then it changes no number. When a deal has several
risks, only the one with the most effect counts.

## See it per deal

**Deals behind the weighted figure**, under the forecast's answer, states the
weighted figure. It lists each open deal. It shows the probability typed on its
stage and the closed deals behind it. It shows the probability used, any risk,
and the weighted amount.

With **Show individual performance** off, the list holds only your own deals,
but the weighted figure still counts your colleagues' deals. When that leaves a
deal out, the panel says so. A shared frozen forecast says the same in its CSV
file, in the column `contributions_limited`. If you own none of its deals, the
file holds one row that says only that.

## The range beside the landing

The **Landing** card on the forecast shows a line such as "8 in 10 outcomes
between €610,000 and €880,000". **Expected finish** in Sales review shows the same
line.

To find the two amounts, the forecast runs the period many times. Each
time, every open deal is won or lost at the probability the forecast uses for
it, with its risk. Money already won is always in. In at least 8 of every 10 runs,
the period ends between the two amounts. At most one run in 10 ends below the low
amount, and at most one ends above the high amount.

The range takes each probability in use as right. It does not check it. The landing
figure is worked out in a different way, so it can sit outside the range. While
the forecast corrects itself, a stage with few closed deals has a probability
the forecast is not so sure of. That makes the range wider.

On both cards, **Evidence** says what the range rests on. The card's other
line, such as the won and still-to-come amounts, moves there too. On the forecast,
**Open** on the **Landing** card goes to **Deals behind the weighted figure**.
That list holds the deals the range was built from, each with its probability. A frozen
forecast keeps the range it showed that day.

## Switch it off

An admin can switch all of this off under **Settings → Company profile →
Forecast**, with **Let the forecast correct itself**. Off, every deal counts at
the probability typed on its stage, as it did before. The forecast still shows a
range, built from the typed probabilities.
