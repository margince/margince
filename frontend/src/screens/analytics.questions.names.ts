// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { useT } from "../i18n";
import { valueLabel } from "./analytics.questions.values";
import { fieldReference } from "./analytics.questions.vocab";
import { useReferenceOptions } from "./filterreference";

type IdLabels = components["schemas"]["AnalyticsIdLabels"];

export const shortId = (id: string) => `${id.slice(0, 8)}…`;

/**
 * Names for the ids in an answer. A company or a project is named by the
 * answer itself (`labels`, read under the reader's grants), so a table of a
 * hundred companies costs no request per row; seats, stages and pipelines come
 * from the roster and configuration lists the Filters builder offers. An id
 * nothing names keeps a short id. An enumerated value takes the word the
 * product already uses for it.
 */
export function useValueNamer(
  entity: string,
  fields: readonly string[],
  labels: IdLabels | undefined,
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
  const listed = new Map<string, string>();
  for (const option of [
    ...users.options,
    ...stages.options,
    ...pipelines.options,
  ]) {
    listed.set(option.value, option.label);
  }
  // A list this answer does not need is never read, and a query never read
  // stays pending; only a list in use can be "still loading".
  const pending =
    (refs.has("app_user") && users.loading) ||
    (refs.has("stage") && stages.loading) ||
    (refs.has("pipeline") && pipelines.loading);
  // A company or project is named by the answer; the rest by the lists.
  const idName = (field: string, id: string, reference: string) =>
    reference === "company" || reference === "project"
      ? labels?.[field]?.[id]
      : listed.get(id);
  return (field: string, value: unknown): string => {
    if (value == null) {
      return t("analytics.q.notSet");
    }
    if (typeof value === "boolean") {
      return t(value ? "analytics.q.yes" : "analytics.q.no");
    }
    const text = String(value);
    const reference = fieldReference(field);
    if (reference === undefined) {
      return valueLabel(t, entity, field, text) ?? text;
    }
    return (
      idName(field, text, reference) ??
      (pending ? t("common.loading") : shortId(text))
    );
  };
}

export type Namer = ReturnType<typeof useValueNamer>;
