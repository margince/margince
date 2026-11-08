// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { EntityRef } from "../../components/entityref";
import { type Fact, FactList } from "../../design-system/factlist";
import { Panel, PanelBody } from "../../design-system/panel";
import { SurfaceState } from "../../design-system/surfacestate";
import { formatMoneyWhole } from "../../format/format";
import { formatDayMonthNearYear } from "../../format/format.dates";
import { UTC_ZONE } from "../../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { useDeal } from "../../lib/dealread";
import { usePipeline } from "../../lib/deals.queries";

type Deal = NonNullable<ReturnType<typeof useDeal>["data"]>;

/** The deal a row is about: what it is worth, where it stands, when it closes. */
export function DealPanel({
  id,
  label,
}: Readonly<{ id: string; label?: string }>) {
  const t = useT();
  const read = useDeal(id);
  const name = read.data?.name ?? label;
  const state = read.isPending
    ? "loading"
    : read.isError
      ? "failed"
      : ("ready" as const);
  return (
    <Panel
      title={
        name ? (
          <EntityRef kind="deal" id={id} name={name} />
        ) : (
          t("worklist.pane.deal")
        )
      }
    >
      <PanelBody>
        <SurfaceState
          state={state}
          emptyLabel={t("worklist.pane.nothing")}
          loadingLabel={t("worklist.pane.loading")}
          detail={{ onRetry: () => void read.refetch() }}
        >
          {read.data && <DealFacts deal={read.data} />}
        </SurfaceState>
      </PanelBody>
    </Panel>
  );
}

function DealFacts({ deal }: Readonly<{ deal: Deal }>) {
  const t = useT();
  const { locale } = useLocale();
  const pipeline = usePipeline(deal.pipeline_id);
  const stage = pipeline.data?.stages?.find(
    (candidate) => candidate.id === deal.stage_id,
  );
  return (
    <FactList facts={dealFacts(deal, stage?.name, t, locale, new Date())} />
  );
}

// A fact the deal does not carry is left out rather than drawn blank.
function dealFacts(
  deal: Deal,
  stageName: string | undefined,
  t: Translator,
  locale: Locale,
  now: Date,
): Fact[] {
  const facts: Fact[] = [];
  // A withheld amount says so, so it never reads as a deal with no price.
  if (deal.masked_fields?.includes("amount_minor")) {
    facts.push({
      key: "value",
      term: t("create.amount"),
      value: t("record.notShown"),
    });
  } else if (deal.amount_minor != null && deal.currency) {
    facts.push({
      key: "value",
      term: t("create.amount"),
      value: formatMoneyWhole(deal.amount_minor, deal.currency, locale),
    });
  }
  if (stageName) {
    facts.push({
      key: "stage",
      term: t("create.stage"),
      value: stageName,
    });
  }
  if (deal.expected_close_date) {
    // A close date is a calendar day, so it is read in UTC and never shifts.
    facts.push({
      key: "close",
      term: t("create.expectedClose"),
      value: formatDayMonthNearYear(
        deal.expected_close_date,
        locale,
        UTC_ZONE,
        now,
      ),
      note: deal.close_date_provisional
        ? t("deal.strip.close.provisional")
        : undefined,
    });
  }
  return facts;
}
