import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { DealStatusBadge } from "../../components/dealstatusbadge";
import { Badge } from "../../design-system/atoms";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { Panel, PanelBody } from "../../design-system/panel";
import { StatCard } from "../../design-system/statcard";
import { StatStrip } from "../../design-system/statstrip";
import { SurfaceState, sectionState } from "../../design-system/surfacestate";
import {
  formatDayMonth,
  formatMoneyOrAbsent,
  formatNumber,
} from "../../format/format";
import { type Locale, useLocale, useT } from "../../i18n";
import { dealsFilteredBy } from "../../lib/dealsaddress";
import { navigate } from "../../lib/router";
import { useViewerZone } from "../../lib/viewerzone";
import type { Company360 } from "./company360";
import "./company360.css";

type Deal360 = components["schemas"]["Company360Deal"];

/** DealsCard lists the open deals plus the two lifetime figures. */
export function DealsCard({
  view,
  actions,
  extra,
  loading = false,
}: Readonly<{
  view?: Company360;
  // The verbs that change this section, rendered under it. Absent on an
  // archived record, which takes no new deals.
  actions?: ReactNode;
  // Whatever else belongs beside this account's deals. The Deals tab hands in
  // the last offer here rather than as a second card. Both start from "this
  // account's open deals", so they read as one section.
  extra?: ReactNode;
  // The composite read's own pending flag (see sectionState). The Deals tab
  // gates its skeleton on `!view && !failed`, so `view` is defined here.
  // It is passed anyway, so the card is correct without a caller's guard.
  loading?: boolean;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const deals = view?.deals;
  const state = sectionState(
    view,
    "deals",
    Boolean(deals),
    deals?.data.length ?? 0,
    loading,
  );
  const present = state === "ready" || state === "empty";
  return (
    <Panel
      title={t("co.deals.title")}
      titleAction={present ? actions : undefined}
    >
      {deals && present && (
        <PanelBody>
          <DealFigures
            deals={deals}
            lostHref={dealsFilteredBy("company_id", view.company.id, {
              status: "lost",
            })}
          />
        </PanelBody>
      )}
      {state === "ready" && (
        <DataTable
          bleed
          fold
          label={t("co.deals.title")}
          columns={dealColumns(t, locale, zone)}
          rows={deals?.data ?? []}
          rowKey={(deal) => deal.deal_id}
        />
      )}
      {state === "empty" && (
        <PanelBody>
          <p className="surfacestate-empty">{t("co.deals.empty")}</p>
        </PanelBody>
      )}
      {present && extra}
      {!present && (
        <PanelBody>
          <SurfaceState
            state={state}
            emptyLabel={t("co.deals.empty")}
            loadingLabel={t("co.deals.title")}
          >
            {null}
          </SurfaceState>
        </PanelBody>
      )}
    </Panel>
  );
}

// The account's deals in four figures: what is open and what it is worth,
// what was won to date, and how many were lost, which opens those deals. A
// sum is stated only in one currency rather than adding unlike money.
function DealFigures({
  deals,
  lostHref,
}: Readonly<{
  deals: NonNullable<Company360["deals"]>;
  lostHref: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const won = deals.won_lifetime;
  const amounts = deals.data.flatMap((deal) =>
    deal.amount?.amount_minor == null ? [] : [deal.amount],
  );
  const currencies = new Set(amounts.map((amount) => amount.currency));
  const [currency] = currencies;
  const openValue =
    currencies.size === 1 && currency
      ? formatMoneyOrAbsent(
          amounts.reduce((sum, amount) => sum + (amount.amount_minor ?? 0), 0),
          currency,
          locale,
        )
      : t("contact.deals.noValue");
  return (
    <StatStrip density="plate" label={t("contact.deals.figures")}>
      <StatCard
        label={t("contact.deals.openCount")}
        value={formatNumber(deals.data.length, locale)}
      />
      <StatCard label={t("contact.deals.openValue")} value={openValue} />
      <StatCard
        label={t("co.deals.wonLifetime")}
        value={formatMoneyOrAbsent(won?.amount_minor, won?.currency, locale)}
      />
      <StatCard
        label={t("co.deals.lost")}
        value={formatNumber(deals.lost_count, locale)}
        href={lostHref}
      />
    </StatStrip>
  );
}

// The design's five columns, each from a field the 360 row carries. A deal
// with no amount or no close date leaves its cell empty rather than inventing
// a zero or a date.
function dealColumns(
  t: ReturnType<typeof useT>,
  locale: Locale,
  zone: string,
): DataTableColumn<Deal360>[] {
  return [
    {
      key: "name",
      header: t("co.deals.col.deal"),
      grow: true,
      render: (deal) => (
        <button
          type="button"
          className="co-rowlink"
          onClick={() => navigate({ screen: "deals", id: deal.deal_id })}
        >
          {deal.name}
        </button>
      ),
    },
    {
      key: "stage",
      header: t("co.deals.col.stage"),
      render: (deal) => deal.stage_name ?? t("co.deals.noStage"),
    },
    {
      key: "value",
      header: t("co.deals.col.value"),
      align: "end",
      render: (deal) =>
        deal.amount?.amount_minor == null
          ? ""
          : formatMoneyOrAbsent(
              deal.amount.amount_minor,
              deal.amount.currency,
              locale,
            ),
    },
    {
      key: "close",
      header: t("co.deals.col.close"),
      render: (deal) =>
        deal.expected_close_date
          ? formatDayMonth(deal.expected_close_date, locale, zone)
          : "",
    },
    {
      key: "status",
      header: t("co.deals.col.status"),
      fold: "end",
      render: (deal) => (
        <span className="co-row-meta">
          <DealStatusBadge status={deal.status} />
          {deal.stalled && (
            <Badge tone="warning">{t("deal.stalledBadge")}</Badge>
          )}
        </span>
      ),
    },
  ];
}
