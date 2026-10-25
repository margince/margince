import { EntityRef, type OwnerNaming } from "../../components/entityref";
import { ownerColumn, tagsColumn } from "../../components/recordlist";
import { Badge } from "../../design-system/atoms";
import { CellStrip, type ListColumn } from "../../design-system/listtable";
import type { BoardMoneyColumn } from "../../design-system/pipelineboard";
import { FieldGuard } from "../../design-system/rbac";
import { formatDate, formatDuration, formatMoney } from "../../format/format";
import type { Locale, useT } from "../../i18n";
import type { StageTotals } from "../../lib/deals.queries";
import { toBoardDeal } from "./boarddeal";
import { closeDayOf } from "./closeday";
import type { CompanyNaming } from "./dealcompanymarks";
import { lastMailColumn } from "./dealmailaside";
import type { Deal, Stage } from "./deals";

export function buildColumns(
  stages: Stage[],
  deals: Deal[],
  totals: Map<string, StageTotals>,
  naming: CompanyNaming,
  // What to say where a stage total would go when the totals query was not
  // asked at all (see measuresTheSamePopulation). Undefined when totals are in
  // play, which is the ordinary case.
  totalsWithheld?: string,
  owners?: OwnerNaming,
): BoardMoneyColumn[] {
  return [...stages]
    .sort((a, b) => a.position - b.position)
    .map((stage) => {
      const stageDeals = deals.filter((deal) => deal.stage_id === stage.id);
      const stageTotals = totals.get(stage.id);
      return {
        stage: stage.id,
        label: stage.name,
        probabilityPct: stage.win_probability,
        // No totals row yet: the report is still in flight, or this stage was
        // not in it. Either way the figure is unknown rather than zero, and the
        // column draws it as absent while still stating the count below.
        rawMinor: stageTotals?.baseMinor ?? null,
        weightedMinor: stageTotals?.weightedBaseMinor ?? null,
        currency: stageTotals?.currency ?? null,
        deals: stageDeals.map((deal) => toBoardDeal(deal, naming, owners)),
        // The true count, not the loaded page's. It falls back to the page
        // count while totals are still loading, so the column shows some
        // number rather than a misleading 0.
        count: stageTotals?.count ?? stageDeals.length,
        asOf: stageTotals?.asOf ?? null,
        timezone: stageTotals?.timezone ?? null,
        unpricedCount: stageTotals?.unpricedCount ?? null,
        sumHidden: totalsWithheld !== undefined,
        sumHiddenReason: totalsWithheld,
      };
    });
}

/**
 * One of a deal's company references, as a table cell.
 *
 * Three readings, and only the last of them is blank. Withheld reads as
 * withheld: the wire sends the id as null and names the field in
 * `masked_fields`. So the null is a refusal rather than an absence, and a cell
 * that drew nothing would state the opposite. A reference the reader may see
 * resolves to the company's name. A deal that names no company has an empty
 * cell, which is the one case where empty is the truth.
 *
 * `asText` because the row is already the link to the deal. A control nested
 * inside one is invalid markup, and the second route would go where the first
 * one already goes.
 */
function CompanyCell({
  deal,
  field,
}: Readonly<{ deal: Deal; field: "company_id" | "partner_company_id" }>) {
  if (deal.masked_fields?.includes(field)) {
    return <FieldGuard mode="masked" />;
  }
  const id = deal[field];
  if (!id) {
    return null;
  }
  return <EntityRef kind="company" id={id} asText />;
}

// The amount's three readings, in one place because both the list and the
// board draw them. Withheld is not empty: reading a refused amount as an
// unpriced deal is the defect this cell exists to prevent.
function AmountCell({
  deal,
  locale,
}: Readonly<{ deal: Deal; locale: Locale }>) {
  if (deal.masked_fields?.includes("amount_minor")) {
    return <FieldGuard mode="masked" />;
  }
  if (deal.amount_minor == null || !deal.currency) {
    return null;
  }
  return (
    <span className="t-num">
      {formatMoney(deal.amount_minor, deal.currency, locale)}
    </span>
  );
}

// The table-view column set, module-level because DealsScreen is already at
// the cognitive-complexity ceiling.
export function dealColumns(
  t: ReturnType<typeof useT>,
  locale: Locale,
  recordZone: string,
  stageName: Map<string, string>,
): ListColumn<Deal>[] {
  return [
    {
      key: "name",
      header: t("contacts.name"),
      cell: (deal) => deal.name,
      // Alphabetical, which a list of deals had no way to offer until the
      // API's sort vocabulary took the columns this list draws.
      sort: "name",
      fixed: true,
    },
    tagsColumn<Deal>(t),
    {
      // The company the deal is with. Withheld is not empty. When the reader
      // may not read that company, the wire sends a null `company_id` and names
      // the field in `masked_fields`. A blank cell cannot be told apart from a
      // deal nobody has linked.
      //
      // Ordered by the company's name, not by the id the field is called
      // after. The sort vocabulary names the reference, and the server decides
      // what ordering it means. A company this reader may not open sorts last,
      // with the rest, because ordering by a name is reading it.
      key: "company",
      header: t("create.relatedCompany"),
      sort: "company_id",
      cell: (deal) => <CompanyCell deal={deal} field="company_id" />,
    },
    {
      // Which partner brought the deal, when one did. Optional: a workspace
      // that runs no partner programme has an empty column. Hiding it per-row
      // is worse in a list than an empty cell, since a column that comes and
      // goes cannot be scanned down.
      key: "partner",
      header: t("deal.partnerCompany"),
      sort: "partner_company_id",
      cell: (deal) => <CompanyCell deal={deal} field="partner_company_id" />,
    },
    {
      key: "stage",
      header: t("deals.stage"),
      // Ordered by the stage's place in its PIPELINE. Alphabetical stages are
      // the funnel shuffled, which is never what somebody sorting by stage
      // means.
      sort: "stage_id",
      cell: (deal) =>
        deal.stage_id ? (stageName.get(deal.stage_id) ?? "") : "",
    },
    {
      key: "amount",
      header: t("deals.amount"),
      numeric: true,
      sort: "amount_minor",
      cell: (deal) => <AmountCell deal={deal} locale={locale} />,
    },
    {
      key: "close",
      header: t("deals.close"),
      holds: "date",
      sort: "close_day",
      cell: (deal) => {
        const day = closeDayOf(deal);
        return day ? formatDate(day, locale, recordZone) : null;
      },
    },
    ownerColumn<Deal>(t),
    {
      // How long since anything happened on this deal. A forecast argument rests
      // on it: an amount with no recent signal behind it is a number nobody can
      // defend. The server already flags the deals that have gone quiet, so
      // the row says so rather than leaving the reader to subtract dates.
      key: "last_signal",
      header: t("deals.lastSignal"),
      numeric: true,
      sort: "last_activity_at",
      cell: (deal) =>
        deal.last_activity_at ? (
          <CellStrip>
            <span>
              {formatDuration(
                Math.max(0, Date.now() - Date.parse(deal.last_activity_at)),
                locale,
              )}
            </span>
            {deal.stalled && (
              <Badge tone="warning">{t("deal.stalledBadge")}</Badge>
            )}
          </CellStrip>
        ) : (
          <span>{t("deals.lastSignalNone")}</span>
        ),
    },
    lastMailColumn(t),
    {
      key: "status",
      header: t("lead.status"),
      cell: (deal) => (
        <Badge tone={dealStatusTone(deal.status)}>{deal.status}</Badge>
      ),
      // By the stored value, so the three groups sit together. Alphabetical
      // (`lost`, `open`, `won`) rather than by lifecycle, because `deal.status`
      // is a text column. The machinery orders by the column, not by a
      // vocabulary this side would have to keep a second copy of.
      sort: "status",
    },
  ];
}

// Won reads success, lost reads danger, an open deal carries no status tone.
// Exported so the partner page's sourced-deals panel reads a deal's status the
// same way the board and the deal record do. A second mapping would render one
// status in two colours on two screens.
export function dealStatusTone(
  status: Deal["status"],
): "success" | "danger" | undefined {
  if (status === "won") {
    return "success";
  }
  if (status === "lost") {
    return "danger";
  }
  return undefined;
}
