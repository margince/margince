import { type ReactNode, useState } from "react";
import { useCan } from "../app/capability";
import { StatCard } from "../design-system/atoms";
import { formatMoney, formatNumber, INTL_LOCALE } from "../format/format";
import { formatElapsed, useNow } from "../format/now";
import { type Locale, useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useProviderKeys } from "./ai-provider-keys";
import { useRouting } from "./ai-routing";
import { type LastCall, useLastCallAt } from "./aicalls";
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

// A token count for a SLOT: "1.2M" rather than "1,204,553".
//
// A month's tokens run to seven figures, and two of them in one value — spent
// against budget — do not fit the one line a stat card holds: the figure clips,
// and a clipped number is a different number rather than a shorter rendering of
// the right one. The Usage tab below carries the exact count, which is where a
// reader who needs the digits is going anyway.
//
// Here rather than in `format/` because this is the only reading in the product
// denominated in tokens; through `INTL_LOCALE` because a locale reaches a
// formatter that way or not at all (format/one-locale.test.ts).
function compactTokens(value: number, locale: Locale): string {
  return new Intl.NumberFormat(INTL_LOCALE[locale], {
    // Below ten thousand the long form is no wider and carries every digit, the
    // same threshold `formatMoneyCompact` abbreviates at, so the two readings
    // on this page step at the same place.
    notation: Math.abs(value) >= 10_000 ? "compact" : "standard",
    maximumFractionDigits: Math.abs(value) >= 10_000 ? 1 : 0,
  }).format(value);
}

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
        spent: compactTokens(budget.spent_tokens, locale),
        budget: compactTokens(budget.monthly_tokens, locale),
      })}
      tone={bandTone(budget.band)}
      meter={{ filled: budget.spent_tokens, total: budget.monthly_tokens }}
      // The money is the ESTIMATE under the budget, and a month nothing priced
      // says so in words: an absent line there reads as a month that cost
      // nothing.
      detail={
        anyPriced
          ? t("aiSettings.spend.estimated", {
              // In FULL, unlike the token figures above it. A month's estimate
              // is a handful of dollars as often as it is thousands, and the
              // compact formatter carries no fraction below ten thousand — it
              // would print forty cents of real spend as "US$0", which is the
              // one claim this product must never make by accident.
              amount: formatMoney(priced, budget.currency ?? "USD", locale),
            })
          : t("aiSettings.spend.notPriced")
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
  // The trace rides a DIFFERENT grant from the keys and answers in states
  // rather than in an instant, so this card never has to guess which silence
  // it is looking at. The grant is the hook's to ask; asking it a second time
  // here would be a second answer to one question.
  const lastCall = useLastCallAt();
  const keys = useProviderKeys(canSeeKeys);
  const routing = useRouting(canSeeKeys);
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
  const bound = routing.data ? boundProviders(routing.data.routing) : null;
  const missing =
    bound === null
      ? null
      : providers.filter((p) => bound.has(p.provider) && !p.configured).length;
  return (
    <StatCard
      label={t("aiSettings.providers.label")}
      value={t("aiSettings.providers.value", {
        count: formatNumber(keyed, locale),
        // Out of the vendors this installation knows about, which is what makes
        // the figure a reading rather than a number a reader has to go and find
        // the denominator for.
        total: formatNumber(providers.length, locale),
      })}
      tone={missing ? "danger" : undefined}
      detail={providersDetail({ missing, lastCall, now, locale }, t)}
    />
  );
}

// What each silence from the call trace is worth saying, and which says
// nothing. A table rather than a ladder: one arm of `LastCall` is one row, so a
// state added to that union and not to this one is a hole a reader can see
// rather than a branch that quietly falls through.
const TRACE_SILENCE: Record<
  Exclude<LastCall["state"], "at">,
  MessageKey | null
> = {
  // Not this reader's to see, which is a fact about them and no evidence about
  // the installation — the card simply does not speak for it.
  withheld: null,
  // Still arriving, and it resolves by waiting.
  unread: null,
  failed: "aiSettings.providers.traceFailed",
  never: "aiSettings.providers.neverCalled",
};

// What qualifies the key count: the bindings that would fail closed, and when a
// vendor was last reached.
//
// ONE line where both are known — the two qualify the same reading, and two grey
// lines read as two readings — so the lead fragment decides the second one's
// case. Nothing to say at all is NO detail rather than an empty one: an empty
// caption draws air under the figure and reads as a line that failed to render.
function providersDetail(
  {
    missing,
    lastCall,
    now,
    locale,
  }: Readonly<{
    missing: number | null;
    lastCall: LastCall;
    now: number;
    locale: Locale;
  }>,
  t: ReturnType<typeof useT>,
): ReactNode {
  const broken = missing
    ? t("aiSettings.providers.missing", {
        count: formatNumber(missing, locale),
      })
    : null;
  if (lastCall.state !== "at") {
    // Its own line rather than a tail, because each of these is a sentence and
    // not a timestamp. ONLY the answered "never" claims the installation has
    // made no call; a trace still arriving or one this reader may not see is a
    // fact about the READ, and both used to fall through to a detail line with
    // nothing in it. A BROKEN read says so rather than going quiet — waiting
    // will not fix it, and silence there reads as a runtime with nothing to
    // report.
    const silence = TRACE_SILENCE[lastCall.state];
    const said = silence ? t(silence) : null;
    if (!broken && !said) {
      return undefined;
    }
    return (
      <>
        {/* ds:ignore a count label in the danger ink, not a message */}
        {broken && <span className="ai-settings-missing">{broken}</span>}
        {said && <span>{said}</span>}
      </>
    );
  }
  const called = t(
    broken
      ? "aiSettings.providers.lastCall"
      : "aiSettings.providers.lastCallOnly",
    { elapsed: formatElapsed(now - lastCall.epochMs, t, locale) },
  );
  return (
    <span>
      {/* ds:ignore a count label in the danger ink, not a message */}
      {broken && <span className="ai-settings-missing">{broken}</span>}
      {broken ? " · " : null}
      {called}
    </span>
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
  routing: NonNullable<ReturnType<typeof useRouting>["data"]>["routing"],
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
