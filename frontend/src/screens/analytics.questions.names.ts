// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueries } from "@tanstack/react-query";
import type { EntityKind } from "../app/entity";
import { useT } from "../i18n";
import { type AnswerRow, fieldReference } from "./analytics.questions.vocab";
import { ENTITY_NAME_KEY, fetchEntityName } from "./entityref";
import { useReferenceOptions } from "./filterreference";

// Two fields name a record the reader may open by its own page; the rest are
// roster or configuration lookups.
const RECORD_FIELD_KIND: Readonly<Record<string, EntityKind>> = {
  company_id: "company",
  partner_company_id: "company",
  project_id: "project",
};

const shortId = (id: string) => `${id.slice(0, 8)}…`;

type RecordRef = Readonly<{ kind: EntityKind; id: string }>;

// Each company or project a set of rows names, once.
function recordRefs(
  fields: readonly string[],
  rows: readonly AnswerRow[],
): RecordRef[] {
  const seen = new Map<string, RecordRef>();
  for (const row of rows) {
    for (const field of fields) {
      const kind = Object.hasOwn(RECORD_FIELD_KIND, field)
        ? RECORD_FIELD_KIND[field]
        : undefined;
      const id = row[field];
      if (kind && typeof id === "string") {
        seen.set(`${kind}:${id}`, { kind, id });
      }
    }
  }
  return [...seen.values()];
}

/**
 * Names for the ids in a set of rows, off the lookups every other surface
 * names them with: the roster and configuration lists the Filters builder
 * offers, and the per-record name read `EntityRef` shares for companies and
 * projects. `useEntityName` reads one id per call and a table has any number,
 * so the record names are read here under the SAME key and query, and one
 * request serves this table and every `EntityRef` beside it.
 */
export function useValueNamer(
  fields: readonly string[],
  rows: readonly AnswerRow[],
) {
  const t = useT();
  const refs = new Set(fields.map(fieldReference));
  const users = useReferenceOptions(
    refs.has("app_user") ? "app_user" : undefined,
  );
  const stages = useReferenceOptions(refs.has("stage") ? "stage" : undefined);
  const pipelines = useReferenceOptions(
    refs.has("pipeline") ? "pipeline" : undefined,
  );
  const records = recordRefs(fields, rows);
  const named = useQueries({
    queries: records.map(({ kind, id }) => ({
      queryKey: [kind, ENTITY_NAME_KEY, id],
      queryFn: () => fetchEntityName(kind, id),
      staleTime: 60_000,
    })),
  });
  const labels = new Map<string, string>();
  for (const option of [
    ...users.options,
    ...stages.options,
    ...pipelines.options,
  ]) {
    labels.set(option.value, option.label);
  }
  records.forEach(({ id }, index) => {
    const name = named[index]?.data;
    if (typeof name === "string" && name.trim() !== "") {
      labels.set(id, name);
    }
  });
  const pending =
    users.loading ||
    stages.loading ||
    pipelines.loading ||
    named.some((q) => q.isPending);
  return (field: string, value: unknown): string => {
    if (value == null) {
      return t("analytics.q.notSet");
    }
    if (typeof value === "boolean") {
      return t(value ? "analytics.q.yes" : "analytics.q.no");
    }
    const text = String(value);
    if (fieldReference(field) === undefined) {
      return text;
    }
    return labels.get(text) ?? (pending ? t("common.loading") : shortId(text));
  };
}

export type Namer = ReturnType<typeof useValueNamer>;
