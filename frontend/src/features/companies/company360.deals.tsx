import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { DealStatusBadge } from "../../components/dealstatusbadge";
import { AppLink } from "../../design-system/applink";
import { Badge } from "../../design-system/atoms";
import { DataTable, type DataTableColumn } from "../../design-system/datatable";
import { Panel, PanelBody } from "../../design-system/panel";
import { SurfaceState, sectionState } from "../../design-system/surfacestate";
import {
  formatDayMonth,
  formatMoneyOrAbsent,
  formatNumber,
} from "../../format/format";
import { type Locale, useLocale, useT } from "../../i18n";
import { dealsFilteredBy } from "../../lib/dealsaddress";
import { useRecordZone } from "../../lib/recordzone";
import { navigate } from "../../lib/router";
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
  const zone = useRecordZone();
  const deals = view?.deals;
  const won = deals?.won_lifetime;
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
      footer={
        deals && (
          <p className="co-row-meta t-caption">
            <span>
              {t("co.deals.wonLifetime")}{" "}
              {formatMoneyOrAbsent(won?.amount_minor, won?.currency, locale)}
            </span>
            {/* The lost count names a set of this company's deals, so it is
                the way into them, and it opens that set. It narrows by `status`
                and not by a lost stage: no single closed-lost stage id exists
                across pipelines, while the deals endpoint reads `status`. */}
            <AppLink
              className="link-button"
              href={dealsFilteredBy("company_id", view.company.id, {
                status: "lost",
              })}
            >
              {t("co.deals.lostCount", {
                count: formatNumber(deals.lost_count, locale),
              })}
            </AppLink>
          </p>
        )
      }
    >
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
