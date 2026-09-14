import { useState } from "react";
import { useCan } from "../app/capability";
import { StatCard } from "../design-system/atoms";
import { formatMoney, formatNumber } from "../format/format";
import { formatElapsed, useNow } from "../format/now";
import { useLocale, useT } from "../i18n";
import { useProviderKeys } from "./ai-provider-keys";
import { useRouting } from "./ai-routing";
import { useLastCallAt } from "./aicalls";
import { bandTone, currentMonth, useAiUsage } from "./aiusage";
import "./ai-settings.css";

// The company's AI as ONE page with five bodies, read in the order the
// questions arrive: WHERE the text goes, WHETHER we can call there, WHAT runs
// unattended, what it spent, and — last, because it is a debugging instrument
// rather than a setting — the per-call trace.
//
// Five bodies rather than six stacked cards. The page had grown to the length of
// four screens, and the two readings an operator opens it for — how much has this
// month cost, and can we still call — were the two furthest from the top. Those
// two now stand in the header above the strip, where they are answered before a
// tab is chosen, and each remaining question is one tab deep instead of one scroll
// deep.
//
// Every card still gates ITSELF, which is why this composes them unconditionally.
// The spend readings and the call trace are reads the server gates on
// automation:update — the AI runtime's spend is treated as operator information,
// so seeing it takes the automation write grant and not any AI-named object — and
// each keeps its place and says so rather than vanishing, because an absent spend
// card claims nothing was spent.
// The two readings an operator opens the AI settings for — how much has this
// month cost, and can we still call.
//
// They used to sit in a header above a five-tab strip, answered before a tab was
// chosen. The strip is gone: each of its tabs is its own page now, so there is
// no header to hold them and no shared address to hold them ABOVE. They keep
// their place instead by riding the two pages they each belong to — spend on
// usage, providers on models — which is where a reader looking for either would
// go anyway.
//
// Exported rather than moved so the queries, the locale formatting and the
// withheld-reading behaviour stay in one file with the cards that share them.

// What this month has cost, in the denomination the runtime actually meters:
// tokens against the monthly ceiling, with the priced estimate under it.
//
// Tokens are the budget and the money is the estimate, in that order, because
// that is which of the two the runtime enforces — the band that degrades a lane
// is drawn on tokens, and a lane never stops because a dollar figure was reached.
// The estimate is priced on read from the workspace's sheet and a call outside it
// carries no price at all, so the money line is absent rather than short when
// nothing in the month priced.
export function SpendStat() {
  const t = useT();
  const { locale } = useLocale();
  // The same gate the endpoint behind `useAiUsage` asks for
  // (ai/usage.go: ai_diagnostics.read). Asking `automation.update` here read
  // the header as withheld for a holder the server would have answered, and
  // rendered the number for an automation editor it would have refused.
  const canSee = useCan("ai_diagnostics", "read");
  // The current month, fixed: the header reads "this month" while the Usage tab
  // below it lets a reader step back through earlier ones, and a header that
  // followed the stepper would stop answering the question it asks.
  const [month] = useState(currentMonth);
  const query = useAiUsage(month, canSee);

  if (!canSee) {
    return (
      <StatCard
        label={t("aiSettings.spend.label")}
        value={t("aiSettings.withheld")}
      />
    );
  }
  const budget = query.data?.budget;
  if (!budget) {
    return (
      <StatCard
        label={t("aiSettings.spend.label")}
        value={readingState(query.isError, t)}
      />
    );
  }
  const priced = (query.data?.days ?? []).reduce(
    (sum, day) =>
      sum +
      day.tasks.reduce(
        (dayTotal, task) => dayTotal + (task.cost_est_minor ?? 0),
        0,
      ),
    0,
  );
  const anyPriced = (query.data?.days ?? []).some((day) =>
    day.tasks.some((task) => task.cost_est_minor !== undefined),
  );
  return (
    <StatCard
      label={t("aiSettings.spend.label")}
      value={t("aiSettings.spend.value", {
        spent: formatNumber(budget.spent_tokens, locale),
        budget: formatNumber(budget.monthly_tokens, locale),
      })}
      tone={bandTone(budget.band)}
      meter={{ filled: budget.spent_tokens, total: budget.monthly_tokens }}
      detail={
        anyPriced
          ? t("aiSettings.spend.estimated", {
              amount: formatMoney(priced, budget.currency ?? "USD", locale),
            })
          : undefined
      }
    />
  );
}

// Whether this installation can call the vendors it has bound.
//
// Two facts, and the second is the one worth the card: how many vendors hold a
// key, and how many the routing document NAMES that hold none. A lane whose
// vendor has no key fails closed at call time, and this is the only place on the
// page where that is visible before the call. The missing half needs both reads,
// so a reader who may see the keys but not the routing gets the count and no
// claim about what is broken — silence rather than a reassuring zero.
export function ProvidersStat() {
  const t = useT();
  const { locale } = useLocale();
  const canSeeKeys = useCan("ai_routing", "read");
  const keys = useProviderKeys(canSeeKeys);
  const routing = useRouting(canSeeKeys);
  const lastCall = useLastCallAt();
  // A minute is the resolution the line reads at, so that is how often it is
  // worth re-rendering for.
  const now = useNow(60_000);

  if (!canSeeKeys) {
    return (
      <StatCard
        label={t("aiSettings.providers.label")}
        value={t("aiSettings.withheld")}
      />
    );
  }
  const providers = keys.data?.providers;
  if (!providers) {
    return (
      <StatCard
        label={t("aiSettings.providers.label")}
        value={readingState(keys.isError, t)}
      />
    );
  }
  const keyed = providers.filter((p) => p.configured).length;
  const bound = routing.data ? boundProviders(routing.data) : null;
  const missing =
    bound === null
      ? null
      : providers.filter((p) => bound.has(p.provider) && !p.configured).length;
  return (
    <StatCard
      label={t("aiSettings.providers.label")}
      value={t("aiSettings.providers.value", {
        count: formatNumber(keyed, locale),
      })}
      tone={missing ? "danger" : undefined}
      detail={
        <>
          {missing ? (
            <span className="ai-settings-missing">
              {t("aiSettings.providers.missing", {
                count: formatNumber(missing, locale),
              })}
            </span>
          ) : null}
          {lastCall !== null && (
            <span>
              {t("aiSettings.providers.lastCall", {
                elapsed: formatElapsed(now - lastCall, t, locale),
              })}
            </span>
          )}
        </>
      }
    />
  );
}

// The vendors the routing document names, chat lanes and the embedding lane
// alike. The embedding lane is in here on purpose: retrieval binds separately
// and can be the only thing pointing at an unkeyed vendor, which is exactly the
// case a reader would otherwise find out about from a failed reindex.
//
// `null` for a body that is not the routing document. `tiers` and `embeddings`
// are both REQUIRED of the response, so the type above says they are there —
// but the type is a promise the WIRE does not keep: nothing validates a 200,
// and reading `Object.values(undefined)` threw, which the error boundary turned
// into the whole settings page saying "this view no longer works". A server too
// old, a projection that lost a field or a proxy answering something else are
// all real ways to get such a body, and none of them should cost a reader the
// page. The caller already draws an unanswered read; this is one.
function boundProviders(
  routing: NonNullable<ReturnType<typeof useRouting>["data"]>,
): Set<string> | null {
  if (routing.tiers === undefined || routing.embeddings === undefined) {
    return null;
  }
  const named = new Set<string>();
  for (const binding of Object.values(routing.tiers)) {
    named.add(binding.provider);
  }
  named.add(routing.embeddings.provider);
  return named;
}

// What a reading says before it has one.
//
// A read that FAILED and a read that has not arrived are different facts, and
// only one of them resolves by waiting: "Reading…" over a failed request is a
// page that says it is still working forever. The failure is stated instead —
// the readings are a glance, and the card that owns each figure carries the
// error and its retry under the tab.
function readingState(failed: boolean, t: ReturnType<typeof useT>): string {
  return failed ? t("aiSettings.unread") : t("aiSettings.pending");
}
