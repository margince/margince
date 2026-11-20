// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { humanizeToken } from "../../format/humanizetoken";
import type { Translator } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { SEARCH_HIT_KIND_KEY } from "../../lib/searchkinds";
import type { ParamField } from "./automations.params";

// The words for every parameter the catalog declares, and for each value a
// closed parameter offers. automations.paramwords.test.ts walks the catalog.

export const PARAM_LABELS: Readonly<Record<string, MessageKey>> = {
  recipient: "auto.renewal.recipient",
  include_imported_changes: "auto.lists.includeImports",
  object: "auto.renewal.object",
  date_field: "auto.renewal.dateField",
  days_before: "auto.renewal.leadTime",
  recurs_yearly: "auto.renewal.annual",
  task_title: "auto.stages.taskTitle",
  pipeline_id: "auto.stages.pipeline",
  stage_ids: "auto.stages.destinations",
  timezone: "auto.deadlineTimezone",
  customer_list_id: "auto.quiet.customerList",
  no_activity_days: "auto.quiet.days",
  check_in_days: "auto.quiet.days",
  due_in_days: "auto.param.dueInDays",
  cap_per_owner: "auto.param.capPerOwner",
  direction: "auto.lists.direction",
  list_id: "auto.lists.watched",
  shortlist_id: "auto.lists.shortlist",
};

export const OPTION_LABELS: Readonly<
  Record<string, Readonly<Record<string, MessageKey>>>
> = {
  recipient: {
    record_owner: "auto.renewal.recordOwner",
    authorizer: "auto.renewal.authorizer",
  },
  direction: {
    entered: "auto.lists.direction.entered",
    left: "auto.lists.direction.left",
    either: "auto.lists.direction.either",
  },
  object: SEARCH_HIT_KIND_KEY,
};

// What the submit button says while a required picker is still empty. A
// required parameter without its own sentence names its label instead.
const CHOOSE: Readonly<Record<string, MessageKey>> = {
  date_field: "auto.needs.dateField",
  object: "auto.needs.object",
  recipient: "auto.needs.recipient",
  pipeline_id: "auto.needs.pipeline",
  stage_ids: "auto.needs.stages",
  list_id: "auto.needs.watched",
  shortlist_id: "auto.needs.shortlist",
  customer_list_id: "auto.needs.customerList",
};

export function paramLabel(field: ParamField, t: Translator): string {
  const key = PARAM_LABELS[field.key];
  return key ? t(key) : humanizeToken(field.key);
}

export function optionLabel(
  field: ParamField,
  value: string,
  t: Translator,
): string {
  const key = OPTION_LABELS[field.key]?.[value];
  return key ? t(key) : humanizeToken(value);
}

// A typed box has always refused an empty value; a picker does when the
// schema says so.
export function isRequired(field: ParamField): boolean {
  return field.required || field.kind === "integer" || field.kind === "string";
}

function isEmpty(field: ParamField, value: string): boolean {
  if (field.kind === "boolean") return false;
  return value.trim() === "" || (field.kind === "stages" && value === "[]");
}

/** Why the form cannot be sent yet: the first required parameter still empty. */
export function missingParam(
  fields: readonly ParamField[],
  values: Readonly<Record<string, string>>,
  t: Translator,
): string | undefined {
  const empty = fields.find(
    (field) => isRequired(field) && isEmpty(field, values[field.key] ?? ""),
  );
  if (!empty) return undefined;
  const key = CHOOSE[empty.key];
  return key
    ? t(key)
    : t("common.fieldRequiredNamed", { field: paramLabel(empty, t) });
}
