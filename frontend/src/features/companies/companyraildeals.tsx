import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { StagePips } from "../../design-system/stagepips";
import { SurfaceState, sectionState } from "../../design-system/surfacestate";
import {
  formatDate,
  formatMoneyOrAbsent,
  formatNumber,
} from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { useRecordZone } from "../../lib/recordzone";
import { routeHash } from "../../lib/router";
import { usePipelines } from "../deals/pipelines.queries";
import { NewDealAction } from "./companyactions";
// The row shapes this file draws live in company360.css, imported here
// because a sibling having loaded it first is not something to assume.
import "./company360.css";
import { useCompanyReadOnlyReason } from "./companyheader";
import {
  AllRowsLink,
  RAIL_ROW_LIMIT,
  SectionTitle,
  sectionAnswered,
  wholeCount,
} from "./companyrailshared";

type Company360 = components["schemas"]["Company360"];
type Company = components["schemas"]["Company"];
type Deal = components["schemas"]["Company360Deal"];
type Pipeline = components["schemas"]["Pipeline"];

/**
 * DealsSection is the account's open pipeline on the overview. It lists the
 * top RAIL_ROW_LIMIT deals with their stage and worth, then what was won.
 * The 360 lists open deals only and counts closed ones, so nothing here
 * filters on `status` again. "Top" puts a flagged deal first, then the money.
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
  const plural = usePlural();
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
  // Read only for rows to place: a withheld or empty section has no stage.
  const pipelines = usePipelines(state === "ready");
  // Closed history vs. never having had a deal: the two empty accounts read
  // differently. One has simply not started; the other stands between cycles.
  const hasClosedHistory = Boolean(
    deals &&
      ((deals.won_lifetime?.amount_minor ?? 0) > 0 || deals.lost_count > 0),
  );
  return (
    <Panel
      title={
        <SectionTitle
          title={t("co.rail.deals.title")}
          count={
            answered && count != null
              ? plural("co.deals.open", count, {
                  count: formatNumber(count, locale),
                })
              : undefined
          }
        />
      }
      titleAction={
        state === "ready" ? (
          <AllRowsLink
            label={t("co.deals.all")}
            onOpen={() => onTab("deals")}
          />
        ) : undefined
      }
    >
      {state === "ready" ? (
        <PanelBody>
          <ul className="co-deal-rows">
            {rows.slice(0, RAIL_ROW_LIMIT).map((deal) => (
              <li key={deal.deal_id}>
                <DealSectionRow
                  deal={deal}
                  stage={stagePlace(pipelines.data, deal.stage_id)}
                />
              </li>
            ))}
          </ul>
          <WonToDate deals={deals} />
        </PanelBody>
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
          <WonToDate deals={deals} />
        </PanelBody>
      )}
    </Panel>
  );
}

// What the account has won to date, under its open deals; nothing before the
// first win.
function WonToDate({ deals }: Readonly<{ deals: Company360["deals"] }>) {
  const plural = usePlural();
  const { locale } = useLocale();
  if (!deals || deals.won_count === 0) {
    return null;
  }
  return (
    <p className="co-deal-won t-caption">
      {plural("co.deals.won", deals.won_count, {
        count: formatNumber(deals.won_count, locale),
        amount: formatMoneyOrAbsent(
          deals.won_lifetime?.amount_minor,
          deals.won_lifetime?.currency,
          locale,
        ),
      })}
    </p>
  );
}

type StagePlace = { at: number; of: number };

// A stage's place among its pipeline's open stages, or nothing while the
// pipelines are unread or the stage is not among them.
function stagePlace(
  pipelines: readonly Pipeline[] | undefined,
  stageId: string | null | undefined,
): StagePlace | undefined {
  const pipeline = pipelines?.find((one) =>
    (one.stages ?? []).some((stage) => stage.id === stageId),
  );
  const open = (pipeline?.stages ?? [])
    .filter((stage) => stage.semantic === "open")
    .sort((a, b) => a.position - b.position);
  const at = open.findIndex((stage) => stage.id === stageId);
  return at < 0 ? undefined : { at: at + 1, of: open.length };
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
        <Button variant="ghost" onClick={() => onTab("deals")}>
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

// The name links to the deal; its close date and any flag sit under it, its
// stage and amount beside it.
function DealSectionRow({
  deal,
  stage,
}: Readonly<{ deal: Deal; stage: StagePlace | undefined }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const closes = deal.expected_close_date
    ? t("co.work.closes", {
        date: formatDate(deal.expected_close_date, locale, recordZone),
      })
    : t("co.rail.deals.noCloseDate");
  const note = [dealFlag(deal, t), closes].filter(Boolean).join(" · ");
  const stageName = deal.stage_name ?? t("co.deals.noStage");
  return (
    <div className="co-deal-row">
      <div className="co-deal-name">
        <a href={routeHash({ screen: "deals", id: deal.deal_id })}>
          {deal.name}
        </a>
        <span className="t-caption">{note}</span>
      </div>
      {stage ? (
        <StagePips
          at={stage.at}
          of={stage.of}
          stalled={deal.stalled}
          label={stageName}
        />
      ) : (
        <span className="co-deal-stage">{stageName}</span>
      )}
      <span className="t-num co-deal-amount">
        {formatMoneyOrAbsent(
          deal.amount?.amount_minor,
          deal.amount?.currency,
          locale,
        )}
      </span>
    </div>
  );
}
