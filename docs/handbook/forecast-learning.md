<!-- prose:plain -->
<!-- describes: backend/internal/modules/forecasting/chance.go backend/internal/compose/forecastchances.go backend/internal/modules/assurance/risk.go frontend/src/features/analytics/analytics.forecast.deals.tsx frontend/src/features/settings/settings.stages.tsx -->
# How the forecast learns

The weighted forecast counts each open deal at a probability. This page says
where that probability comes from and how it changes as your deals close.

## Each stage starts where you typed it

Each stage starts at the win probability typed on it under **Settings →
Stages**. With no closed deal from a stage yet, the forecast uses that
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

**Settings → Stages** shows this beside each stage, for example "Typed 90% · 40
recorded deals from here closed, 55% won · Forecast uses 62%".

## Risks on a deal

The forecast marks four risks on an open deal:

- the close date moved more than twice in 90 days;
- no next step is written down;
- the customer has gone quiet;
- the deal has been in its stage longer than nine of every 10 won deals were.

A risk is shown at once. It changes a deal's weight only later. First, frozen
forecasts of your own installation must close. They must show what the risk did
to deals that had it. Until then it changes no number. When a deal has several
risks, only the one with the most effect counts.

## See it per deal

**Deals behind the weighted figure**, under the forecast's answer, lists each
open deal. It shows the probability typed on its stage and the closed deals
behind it. It shows the probability used, any risk, and the weighted amount.

## Switch it off

An admin can switch all of this off under **Settings → Installation →
Forecast**, with **Let the forecast correct itself**. Off, every deal counts at
the probability typed on its stage, as it did before.
