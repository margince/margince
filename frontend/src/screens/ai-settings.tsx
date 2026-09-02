import { useState } from "react";
import { useCan } from "../app/capability";
import { StatCard } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { RecordTabs } from "../design-system/recordtabs";
import { formatMoney, formatNumber } from "../format/format";
import { formatElapsed, useNow } from "../format/now";
import { useLocale, useT } from "../i18n";
import { AiCertificationCard } from "./ai-certification";
import { AiHealthCard } from "./ai-health";
import { AiProviderKeysCard, useProviderKeys } from "./ai-provider-keys";
import { AiRoutingCard, useRouting } from "./ai-routing";
import { AiCallsCard, useLastCallAt } from "./aicalls";
import { AiUsageCard, bandTone, currentMonth, useAiUsage } from "./aiusage";
import { AutomationsAdmin } from "./automations";
import { ModelCostsCard } from "./rates";
import "./ai-settings.css";

// The organization's AI as ONE page with five bodies, read in the order the
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
const AI_TABS = [
  "routing",
  "providers",
  "automations",
  "usage",
  "logs",
] as const;

type AiTab = (typeof AI_TABS)[number];

export function AiSettingsTab() {
  const t = useT();
  const [tab, setTab] = useState<AiTab>("routing");
  // A routing draft is a document held in the card, not a field that saves
  // itself, and the strip above it is a place a reader moves rather than a
  // navigation the app's own unsaved guard can see: the guard watches ADDRESSES,
  // and every tab here shares one. So the shell asks, and a switch that would
  // discard work waits for an answer.
  const [routingDirty, setRoutingDirty] = useState(false);
  const [pending, setPending] = useState<AiTab | null>(null);

  const choose = (next: AiTab) => {
    if (next === tab) {
      return;
    }
    if (tab === "routing" && routingDirty) {
      setPending(next);
      return;
    }
    setTab(next);
  };

  return (
    <>
      <header className="ai-settings-head">
        <p className="settings-panel-sub">{t("aiSettings.sub")}</p>
        <div className="ai-settings-stats">
          <SpendStat />
          <ProvidersStat />
        </div>
      </header>
      <RecordTabs
        options={AI_TABS}
        value={tab}
        onChange={choose}
        label={t("aiSettings.tabs")}
        labels={{
          routing: t("aiSettings.tab.routing"),
          providers: t("aiSettings.tab.providers"),
          automations: t("aiSettings.tab.automations"),
          usage: t("aiSettings.tab.usage"),
          logs: t("aiSettings.tab.logs"),
        }}
      />
      {/* One body at a time, mounted only while it is open: each is a query
          against a different endpoint, and keeping four of them warm behind a
          strip nobody is looking at spends the installation's read budget on
          screens that are not on screen. */}
      {tab === "routing" && (
        <>
          <AiRoutingCard
            onDirtyChange={setRoutingDirty}
            onPriceSheet={() => choose("usage")}
          />
          {/* How well the models bound above actually do each job. It belongs
              with the BINDING rather than with the lanes' health, because it is
              a reading OF the binding: change a model here and what is known
              about the model you chose is in the same place, not a tab away. */}
          <AiCertificationCard />
        </>
      )}
      {tab === "providers" && (
        <>
          <AiProviderKeysCard />
          {/* Whether the vendors above are actually ANSWERING. It belongs with
              the credentials rather than with the bindings, because the three
              readings are one story told in order — which vendor a lane names,
              whether we hold a key for it, whether it replied — and the last two
              are the pair an operator checks together when a lane goes quiet. */}
          <AiHealthCard />
        </>
      )}
      {tab === "automations" && <AutomationsAdmin />}
      {tab === "usage" && (
        <>
          <AiUsageCard />
          <ModelCostsCard />
        </>
      )}
      {tab === "logs" && <AiCallsCard />}
      <ConfirmModal
        open={pending !== null}
        onClose={() => setPending(null)}
        title={t("aiSettings.discardTitle")}
        confirmLabel={t("aiSettings.discard")}
        confirmVariant="danger"
        onConfirm={() => {
          if (pending) {
            setRoutingDirty(false);
            setTab(pending);
          }
          setPending(null);
        }}
      >
        {t("aiSettings.discardBody")}
      </ConfirmModal>
    </>
  );
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
function SpendStat() {
  const t = useT();
  const { locale } = useLocale();
  const canSee = useCan("automation", "update");
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
function ProvidersStat() {
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
function boundProviders(
  routing: NonNullable<ReturnType<typeof useRouting>["data"]>,
): Set<string> {
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
