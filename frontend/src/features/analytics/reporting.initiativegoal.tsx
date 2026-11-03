// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { DateInput, isoOrBlank } from "../../design-system/dateinput";
import { Field, Textarea, TextInput } from "../../design-system/field";
import { Select } from "../../design-system/select";
import { useT } from "../../i18n";
import type { SalesInitiative } from "./reporting.initiatives";
import { useMetricNames, useReportingCatalog } from "./reporting.queries";
import {
  type SetupProposal,
  SetupProposalStep,
} from "./reporting.setupproposal";

export type Input = components["schemas"]["SalesInitiativeInput"];
export type Members = components["schemas"]["SalesInitiativeMembers"];
export type MemberRef = components["schemas"]["SalesInitiativeMemberRef"];
export type MetricID = components["schemas"]["ReportingMetricID"];

/** The objectives an initiative may be measured by, in the order offered. */
export const OBJECTIVES: readonly MetricID[] = [
  "qualified_opportunities_created",
  "qualified_pipeline_created",
  "bookings_won",
  "open_pipeline",
];

export type Step = "goal" | "members" | "review";

export type Goal = Readonly<{
  name: string;
  description: string;
  objective: MetricID;
  target: string;
  deadline: string;
  includeFuture: boolean;
}>;

export type Rule = Readonly<{
  product: string;
  industry: string;
  country: string;
}>;

/** The rule as the predicate engine reads it: every set condition, AND-ed. */
export function predicateOf(rule: Rule, recordType: "lead" | "deal") {
  const leaves: { field: string; op: "eq"; value: string }[] = [];
  if (rule.product)
    leaves.push({ field: "product_id", op: "eq", value: rule.product });
  if (rule.industry)
    leaves.push({
      field: recordType === "deal" ? "company_industry" : "industry",
      op: "eq",
      value: rule.industry,
    });
  if (rule.country)
    leaves.push({ field: "company_country", op: "eq", value: rule.country });
  return leaves.length === 0 ? undefined : { and: leaves };
}

export function goalOf(initiative?: SalesInitiative): Goal {
  return {
    name: initiative?.name ?? "",
    description: initiative?.description ?? "",
    objective:
      initiative?.objective_metric ?? "qualified_opportunities_created",
    target:
      initiative?.target_value == null ? "" : String(initiative.target_value),
    deadline: initiative?.deadline ?? "",
    includeFuture: initiative?.include_future ?? true,
  };
}

export function ruleOf(initiative?: SalesInitiative): Rule {
  const tree = initiative?.deal_rule as
    | { and?: { field: string; value: string }[] }
    | undefined;
  const leaves = tree?.and ?? [];
  const value = (field: string) =>
    leaves.find((leaf) => leaf.field === field)?.value ?? "";
  return {
    product: value("product_id"),
    industry: value("company_industry"),
    country: value("company_country"),
  };
}

export function bodyOf(goal: Goal, rule: Rule): Input {
  return {
    name: goal.name.trim(),
    description:
      goal.description.trim() === "" ? null : goal.description.trim(),
    objective_metric: goal.objective,
    target_value: goal.target.trim() === "" ? null : Number(goal.target),
    deadline: goal.deadline,
    include_future: goal.includeFuture,
    deal_rule: predicateOf(rule, "deal") ?? {},
    lead_rule: predicateOf(rule, "lead") ?? {},
  };
}

export function GoalFields({
  goal,
  onChange,
  onProposal,
}: Readonly<{
  goal: Goal;
  onChange: (goal: Goal) => void;
  // A proposal read from the description, for the drawer to apply to the
  // goal and the rule together.
  onProposal: (proposal: SetupProposal) => void;
}>) {
  const t = useT();
  const catalog = useReportingCatalog();
  const metricName = useMetricNames();
  const offered = OBJECTIVES.filter((id) =>
    catalog.data?.metrics.some((metric) => metric.id === id),
  );
  return (
    <div className="form-stack">
      <Field label={t("reporting.initiativeName")}>
        {(field) => (
          <TextInput
            {...field}
            value={goal.name}
            placeholder={t("reporting.initiativeNamePlaceholder")}
            onChange={(event) =>
              onChange({ ...goal, name: event.target.value })
            }
          />
        )}
      </Field>
      <Field
        label={t("reporting.describeInitiative")}
        hint={t("reporting.describeInitiativeHint")}
      >
        {(field) => (
          <Textarea
            {...field}
            value={goal.description}
            rows={3}
            maxLength={2000}
            onChange={(event) =>
              onChange({ ...goal, description: event.target.value })
            }
          />
        )}
      </Field>
      <SetupProposalStep
        site="initiative_setup"
        description={goal.description}
        onApply={onProposal}
      />
      <Field label={t("reporting.objective")}>
        {(field) => (
          <Select
            {...field}
            value={goal.objective}
            options={(offered.length > 0 ? offered : OBJECTIVES).map((id) => ({
              value: id,
              label: metricName(id),
            }))}
            onChange={(value) => {
              const objective = OBJECTIVES.find((id) => id === value);
              if (objective) onChange({ ...goal, objective });
            }}
          />
        )}
      </Field>
      <Field
        label={t("reporting.targetValue")}
        hint={t("reporting.targetHint")}
      >
        {(field) => (
          <TextInput
            {...field}
            inputMode="numeric"
            value={goal.target}
            onChange={(event) =>
              onChange({
                ...goal,
                target: event.target.value.replace(/\D/g, ""),
              })
            }
          />
        )}
      </Field>
      <Field label={t("reporting.deadline")}>
        {(field) => (
          <DateInput
            {...field}
            value={isoOrBlank(goal.deadline)}
            onChange={(event) =>
              onChange({ ...goal, deadline: event.target.value })
            }
          />
        )}
      </Field>
      <p className="t-caption">{t("reporting.nameDoesNotDecide")}</p>
    </div>
  );
}
