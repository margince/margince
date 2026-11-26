// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { formatAnalyticsMoneyOrAbsent } from "../../format/format";
import {
  isMessageKey,
  type Locale,
  type Translator,
  useLocale,
  useT,
} from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { translatedPipeline } from "../../lib/labeltranslation";
import { unwrap } from "../../lib/problem";
import {
  isMoneyField,
  isNativeMoneyField,
  type MoneyFields,
} from "./analytics.questions.money";

// A drill-through's source rows as a reader sees them: which columns show, what
// each is headed, and how each cell reads.

type Derivation = components["schemas"]["ReportDerivation"];

// The drill-through's own words for its columns. Any other column takes the
// builder's name for the same field, and a column with neither is not shown.
const DERIVATION_HEADERS: Readonly<Record<string, MessageKey>> = {
  id: "analytics.q.recordId",
  amount_base_minor: "analytics.unweighted",
  weighted_base_minor: "analytics.weighted",
  amount_minor: "analytics.unweighted",
  currency: "analytics.currency",
  stage_id: "explain.col.stage",
  owner_id: "explain.col.owner",
  pipeline_id: "explain.col.pipeline",
  company_id: "analytics.company",
  partner_company_id: "analytics.field.partner_company_id",
};

// What a source row IS, per report: the label names the record the row stands
// for, and the three delivery reports count projects rather than deals.
const RECORD_NOUN: Readonly<Record<string, MessageKey>> = {
  "projects-by-phase": "analytics.project",
  "project-commitments": "analytics.project",
  "projects-gone-quiet": "analytics.project",
};

// A converted money column names the base currency, because the row's own
// `currency` column beside it names the deal's.
export function derivationHeader(
  col: string,
  report: string,
  frame: Readonly<{
    baseCurrency: string | null;
    money: MoneyFields;
    t: Translator;
  }>,
): string {
  const { baseCurrency, money, t } = frame;
  const key = derivationHeaderKey(col, report);
  const label = key ? t(key) : col;
  return baseCurrency && countsInBase(col, money)
    ? t("explain.col.inCurrency", { label, currency: baseCurrency })
    : label;
}

// The id is noise beside a name, but only once EVERY row has one: a name is
// withheld per row, and dropping the id then blanks that row's only identifier.
// The record and its company lead, whatever order the plan selected. Columns
// past a narrow drawer's edge are the ones a reader never sees.
function derivationHeaderKey(
  col: string,
  report: string,
): MessageKey | undefined {
  if (col === "label") return RECORD_NOUN[report] ?? "explain.col.record";
  const field = `analytics.field.${col}`;
  return DERIVATION_HEADERS[col] ?? (isMessageKey(field) ? field : undefined);
}

// The columns a drill-through shows: only those it has words for, so no wire
// name reaches the reader.
export function shownDerivationColumns(
  derivation: Pick<Derivation, "report" | "columns" | "rows">,
): string[] {
  return derivationColumns(derivation).filter(
    (col) => derivationHeaderKey(col, derivation.report) !== undefined,
  );
}

export function derivationColumns(
  derivation: Pick<Derivation, "columns" | "rows">,
): string[] {
  const rows = derivation.rows ?? [];
  const everyRowNamed =
    derivation.columns.includes("label") &&
    rows.length > 0 &&
    rows.every((row) => typeof row.label === "string" && row.label !== "");
  const shown = derivation.columns.filter(
    (col) => !everyRowNamed || col !== "id",
  );
  const lead = LEAD_COLUMNS.filter((col) => shown.includes(col));
  return [...lead, ...shown.filter((col) => !lead.includes(col))];
}

const LEAD_COLUMNS = ["label", "id", "company_id"];

// Whether a money column is counted in the base currency, as the report's
// served schema marks it. Never read from the column's name, which a converted
// project value shares with a native amount.
function countsInBase(col: string, money: MoneyFields): boolean {
  return isMoneyField(col, money) && !isNativeMoneyField(col, money);
}

export function derivationCellCurrency(
  col: string,
  row: Record<string, unknown>,
  baseCurrency: string | null,
  money: MoneyFields,
): string | null {
  if (countsInBase(col, money)) {
    return baseCurrency;
  }
  const own = row.currency;
  return typeof own === "string" && own !== "" ? own : null;
}

// Money is stored in minor units, and 500000 printed raw beside €5,000.00 is the
// most misread thing on this screen.
export function renderDerivationCell(
  col: string,
  row: Record<string, unknown>,
  frame: Readonly<{
    baseCurrency: string | null;
    money: MoneyFields;
    locale: Locale;
  }>,
): ReactNode {
  const value = row[col];
  if (value == null) {
    return "";
  }
  if (typeof value === "string" && value !== "") {
    if (col === "pipeline_id")
      return <DerivationPipelineName pipelineId={value} />;
    if (col === "stage_id" && typeof row.pipeline_id === "string") {
      return (
        <DerivationPipelineName pipelineId={row.pipeline_id} stageId={value} />
      );
    }
    if (col === "owner_id") return <EntityRef kind="user" id={value} />;
    if (col === "company_id" || col === "partner_company_id") {
      return <EntityRef kind="company" id={value} />;
    }
  }
  if (isMoneyField(col, frame.money) && typeof value === "number") {
    return formatAnalyticsMoneyOrAbsent(
      value,
      derivationCellCurrency(col, row, frame.baseCurrency, frame.money),
      frame.locale,
    );
  }
  return String(value);
}

function DerivationPipelineName({
  pipelineId,
  stageId,
}: Readonly<{ pipelineId: string; stageId?: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const pipeline = useQuery({
    select: (data) => translatedPipeline(data, locale),
    queryKey: ["pipeline", pipelineId],
    queryFn: async () => {
      return unwrap(
        await api.GET("/pipelines/{id}", {
          params: { path: { id: pipelineId } },
        }),
      );
    },
  });
  if (pipeline.isPending) return <>{t("common.loading")}</>;
  if (pipeline.isError) return <>{t("common.error")}</>;
  return (
    <>
      {stageId
        ? (pipeline.data?.stages?.find((stage) => stage.id === stageId)?.name ??
          t("common.empty"))
        : pipeline.data?.name}
    </>
  );
}
