// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { isMessageKey, type Translator, useLocale, useT } from "../../i18n";
import { isRecord } from "../../lib/problembody";
import { renderDerivationCell } from "./analytics.explain.cells";
import { NO_MONEY } from "./analytics.questions.money";
import {
  analyticsFieldLabel,
  ENTITY_LABEL_KEY,
  entityLabel,
  MEASURE_FNS,
  type MeasureFn,
  measureLabel,
} from "./analytics.questions.vocab";

type Derivation = components["schemas"]["ReportDerivation"];

type Condition = Readonly<{ field: string; label: string }>;

export type DefinitionWords = Readonly<{
  sentence: string;
  // The group or filter the figure was cut to, each by its field's name.
  conditions: readonly Condition[];
  // The values the conditions bind, by field; a field pinned as unset is absent.
  values: Readonly<Record<string, string>>;
}>;

/**
 * What the explained figure counts, in the reader's language, from the plan
 * the server resolved and the fields the handle pins as unset. Null when the
 * plan names a report, field or aggregate with no words here: leaving that
 * part out would explain a broader figure, so the server's sentence stands.
 */
export function definitionWords(
  t: Translator,
  derivation: Pick<Derivation, "report" | "plan" | "rows">,
  unset: readonly string[],
): DefinitionWords | null {
  const plan = readPlan(derivation.plan);
  if (!plan || !Object.hasOwn(ENTITY_LABEL_KEY, derivation.report)) return null;
  const measures: string[] = [];
  for (const { fn, field } of plan.aggregates) {
    if (field !== undefined && !fieldLabelled(field)) return null;
    measures.push(measureLabel(t, { fn, field }));
  }
  const values: Record<string, string> = { ...plan.predicates };
  const stage = values.stage_id;
  const firstPipeline = derivation.rows[0]?.pipeline_id;
  if (stage !== undefined && values.pipeline_id === undefined) {
    // A stage is named within its pipeline, which every row of the stage shares.
    if (typeof firstPipeline !== "string") return null;
    values.pipeline_id = firstPipeline;
  }
  const fields = [...Object.keys(plan.predicates), ...unset].sort();
  if (!fields.every(fieldLabelled)) return null;
  return {
    sentence: t("explain.definition", {
      population: entityLabel(t, derivation.report),
      measures: measures.join(", "),
    }),
    conditions: fields.map((field) => ({
      field,
      label: analyticsFieldLabel(t, field),
    })),
    values,
  };
}

function fieldLabelled(field: string): boolean {
  return isMessageKey(`analytics.field.${field}`);
}

type Plan = Readonly<{
  aggregates: readonly { fn: MeasureFn; field?: string }[];
  predicates: Readonly<Record<string, string>>;
}>;

function readPlan(plan: unknown): Plan | null {
  if (!isRecord(plan)) return null;
  const { aggregates, predicates = {} } = plan;
  if (!Array.isArray(aggregates) || aggregates.length === 0) return null;
  const read: { fn: MeasureFn; field?: string }[] = [];
  for (const aggregate of aggregates) {
    if (!isRecord(aggregate)) return null;
    const fn = MEASURE_FNS.find((each) => each === aggregate.fn);
    const { field } = aggregate;
    if (!fn || (field !== undefined && typeof field !== "string")) return null;
    read.push(field === undefined ? { fn } : { fn, field });
  }
  if (!isRecord(predicates)) return null;
  const entries = Object.entries(predicates);
  if (!entries.every(([, value]) => typeof value === "string")) return null;
  return { aggregates: read, predicates: Object.fromEntries(entries) };
}

/** The figure's definition, in the reader's words where the plan allows. */
export function DerivationDefinition({
  derivation,
  unset,
}: Readonly<{ derivation: Derivation; unset: readonly string[] }>) {
  const t = useT();
  const { locale } = useLocale();
  const words = definitionWords(t, derivation, unset);
  if (!words) return <p className="t-sub">{derivation.definition}</p>;
  return (
    <>
      <p className="t-sub">{words.sentence}</p>
      {words.conditions.length > 0 && (
        <p className="t-sub">
          {words.conditions.map(({ field, label }, index) => (
            <span key={field}>
              {index > 0 && " · "}
              {label}:{" "}
              {Object.hasOwn(words.values, field)
                ? renderDerivationCell(field, words.values, {
                    baseCurrency: null,
                    money: NO_MONEY,
                    locale,
                  })
                : t("analytics.q.notSet")}
            </span>
          ))}
        </p>
      )}
    </>
  );
}
