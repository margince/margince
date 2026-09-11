import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { routeHash } from "../app/router";
import { Button, Disclosure } from "../design-system/atoms";
import { PanelBody, PanelRow } from "../design-system/panel";
import { SurfaceState, sectionState } from "../design-system/surfacestate";
import {
  formatDate,
  formatMoneyOrAbsent,
  formatNumber,
} from "../format/format";
import { useLocale, useT } from "../i18n";
import { NewDealAction } from "./companyactions";
// The row and card shapes this file draws — co-rowlink, co-row-meta, co-card —
// live in company360.css. Imported HERE rather than relied on from the rail:
// this file renders wherever it is mounted, and a sibling having loaded the
// stylesheet first is not something it can assume.
import "./company360.css";
import { useCompanyReadOnlyReason } from "./companyheader";
import {
  RAIL_ROW_LIMIT,
  SectionSummary,
  sectionAnswered,
  wholeCount,
} from "./companyrailshared";

// The account's open pipeline at a glance. Extracted from companyrail.tsx so
// that file stays under the length cap; the rail imports it and mounts it in
// the same place it always sat.

type Company360 = components["schemas"]["Company360"];
type Company = components["schemas"]["Company"];
type Deal = components["schemas"]["Company360Deal"];

/**
 * DealsSection is the account's open pipeline at a glance: the top
 * RAIL_ROW_LIMIT deals, each with its stage, expected close, and the deal's
 * own reason for needing attention ahead of everything else about it.
 * `view.deals.data` is already open-only (the 360's own contract — closed
 * deals are reported through `won_lifetime` and `lost_count`, never listed),
 * so nothing here filters on `status` a second time.
 *
 * "Top" means: a deal carrying an attention flag before one without, and the
 * larger amount before the smaller — the row a rep would want surfaced is
 * the one that needs a move or carries the money.
 */
export function DealsSection({
  view,
  loading,
  onTab,
}: Readonly<{
  view?: Company360;
  loading: boolean;
  onTab: (tab: "deals") => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const deals = view?.deals;
  const rows = rankedDeals(deals?.data ?? []);
  const count = wholeCount(deals);
  const state = sectionState(
    view,
    "deals",
    Boolean(deals),
    rows.length,
    loading,
  );
  const answered = sectionAnswered(state);
  // Closed history vs. never having had a deal: the two empty accounts read
  // differently — one has simply not started, the other has already been
  // through a cycle and stands between two of them.
  const hasClosedHistory = Boolean(
    deals &&
      ((deals.won_lifetime?.amount_minor ?? 0) > 0 || deals.lost_count > 0),
  );
  return (
    <Disclosure
      className="co-sect"
      open
      summary={
        <SectionSummary
          title={t("co.rail.deals.title")}
          count={answered ? count : undefined}
        />
      }
    >
      {state === "ready" ? (
        rows
          .slice(0, RAIL_ROW_LIMIT)
          .map((deal) => <DealRailRow key={deal.deal_id} deal={deal} />)
      ) : (
        <PanelBody>
          <SurfaceState
            loadingLabel={t("co.rail.deals.title")}
            state={state}
            emptyLabel={
              hasClosedHistory
                ? t("co.rail.deals.emptyClosedOnly")
                : t("co.rail.deals.empty")
            }
          >
            {null}
          </SurfaceState>
          {state === "empty" && view?.company && (
            <DealsEmptyVerb
              company={view.company}
              betweenCycles={hasClosedHistory}
              onTab={onTab}
            />
          )}
        </PanelBody>
      )}
      {state === "ready" && (
        <div className="card-actions">
          <Button small variant="ghost" onClick={() => onTab("deals")}>
            {count != null
              ? t("co.rail.all", { count: formatNumber(count, locale) })
              : t("co.rail.allUncounted")}
          </Button>
        </div>
      )}
    </Disclosure>
  );
}

// The ONE verb an empty pipeline carries. An account that never started gets
// the create verb when the reader may write; one between cycles, or a reader
// who may not write, gets the way to the Deals tab instead. Never both — two
// verbs under one empty state is a choice the reader has no basis to make.
// Gated on writability the same way TagsSection's own add-tag verb is:
// `useCompanyReadOnlyReason` needs a resolved Company, so this is its
// own component mounted only once one exists, rather than a conditional
// hook call inside DealsSection itself.
function DealsEmptyVerb({
  company,
  betweenCycles,
  onTab,
}: Readonly<{
  company: Company;
  betweenCycles: boolean;
  onTab: (tab: "deals") => void;
}>) {
  const t = useT();
  const readOnlyReason = useCompanyReadOnlyReason(company);
  if (betweenCycles || readOnlyReason) {
    return (
      <div className="card-actions">
        <Button small variant="ghost" onClick={() => onTab("deals")}>
          {t("co.rail.add")}
        </Button>
      </div>
    );
  }
  return (
    <div className="card-actions">
      <NewDealAction
        companyId={company.id}
        companyName={company.display_name}
      />
    </div>
  );
}

// The rail's own ranking: a deal that needs a move outranks one that does
// not, and past that the money decides — but only when every priced deal on
// the account shares one KNOWN currency, because minor units of different or
// unrecorded currencies are not comparable and a raw compare would rank ¥
// over € on digit count. The decision is made ONCE over the whole list, not
// inside the comparator: a pairwise "these two do not compare" while other
// pairs still reorder is a non-transitive comparator, and Array.sort answers
// that with an arbitrary order rather than the server's. When the amounts do
// not compare, the stable sort keeps the server's own order — the one every
// other deal surface shows — past the attention split.
function rankedDeals(rows: readonly Deal[]): Deal[] {
  // Only PRICED deals vote on comparability: an unpriced deal's currency is
  // not a figure anybody ranks, and letting it into the set would stop two
  // priced same-currency deals from ranking on a deal with nothing to rank.
  const currencies = new Set(
    rows.flatMap((deal) =>
      deal.amount?.amount_minor != null && deal.amount.currency
        ? [deal.amount.currency]
        : [],
    ),
  );
  const amountsComparable =
    currencies.size <= 1 &&
    rows.every(
      (deal) => deal.amount?.amount_minor == null || deal.amount.currency,
    );
  const needsMove = (deal: Deal) => (deal.attention || deal.stalled ? 1 : 0);
  return [...rows].sort((a, b) => {
    const moved = needsMove(b) - needsMove(a);
    if (moved !== 0 || !amountsComparable) {
      return moved;
    }
    return (b.amount?.amount_minor ?? 0) - (a.amount?.amount_minor ?? 0);
  });
}

// One flag a deal can carry ahead of its stage and close date: an overdue
// task beats a stall, because a stall is the absence of a reason and an
// overdue task IS one — the same precedence the work list draws its own
// attention line by.
function dealFlag(deal: Deal, t: ReturnType<typeof useT>): string | undefined {
  if (deal.attention) {
    return deal.attention.kind === "overdue_task"
      ? t("co.rail.deals.attentionOverdue")
      : t("co.rail.deals.attentionCommitment");
  }
  return deal.stalled ? t("deal.stalled") : undefined;
}

function DealRailRow({ deal }: Readonly<{ deal: Deal }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const closes = deal.expected_close_date
    ? t("co.work.closes", {
        date: formatDate(deal.expected_close_date, locale, recordZone),
      })
    : t("co.rail.deals.noCloseDate");
  const flag = dealFlag(deal, t);
  const note = [flag, deal.stage_name ?? t("co.deals.noStage"), closes]
    .filter(Boolean)
    .join(" · ");
  return (
    <PanelRow className="co-row">
      <a
        className="co-rowlink co-rowcover"
        href={routeHash({ screen: "deals", id: deal.deal_id })}
      >
        {deal.name}
      </a>
      <span className="t-mono">
        {formatMoneyOrAbsent(
          deal.amount?.amount_minor,
          deal.amount?.currency,
          locale,
        )}
      </span>
      <p className="co-row-meta t-caption">{note}</p>
    </PanelRow>
  );
}
