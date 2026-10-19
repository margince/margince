import type { ReactNode } from "react";
import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { SurfaceState, sectionState } from "../../design-system/surfacestate";
import { formatMoneyOrAbsent, formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { navigate } from "../../lib/router";
import { dealsFilteredBy } from "../deals/dealsaddress";
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
            <a
              className="link-button"
              href={dealsFilteredBy("company_id", view.company.id, {
                status: "lost",
              })}
            >
              {t("co.deals.lostCount", {
                count: formatNumber(deals.lost_count, locale),
              })}
            </a>
          </p>
        )
      }
    >
      {present ? (
        <>
          {(deals?.data ?? []).map((deal) => (
            <DealRow key={deal.deal_id} deal={deal} />
          ))}
          {state === "empty" && (
            <PanelBody>
              <p className="surfacestate-empty">{t("co.deals.empty")}</p>
            </PanelBody>
          )}
          {extra}
        </>
      ) : (
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

function DealRow({ deal }: Readonly<{ deal: Deal360 }>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <PanelRow className="co-row">
      <button
        type="button"
        className="co-rowlink"
        onClick={() => navigate({ screen: "deals", id: deal.deal_id })}
      >
        {deal.name}
      </button>
      <span className="co-row-meta t-caption">
        <span>{deal.stage_name ?? t("co.deals.noStage")}</span>
        {deal.amount?.amount_minor != null && (
          <span className="t-num">
            {formatMoneyOrAbsent(
              deal.amount.amount_minor,
              deal.amount.currency,
              locale,
            )}
          </span>
        )}
        {deal.stalled && <Badge tone="warning">{t("deal.stalledBadge")}</Badge>}
      </span>
    </PanelRow>
  );
}
