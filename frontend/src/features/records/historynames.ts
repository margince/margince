// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueries, useQuery } from "@tanstack/react-query";
import { entityNameObserverQuery } from "../../components/entityref.queries";
import { useLocale } from "../../i18n";
import { translatedLabel } from "../../lib/labeltranslation";
import { useMemberNames } from "../../lib/membernames";
import { pipelinesWithArchivedOptions } from "../../lib/pipelines.queries";
import { isUuid } from "../../lib/uuid";

// What an id stored in a history field points at, read through the reader the
// rest of the page names that kind of record with.
const ID_FIELD_KINDS = new Map<
  string,
  "member" | "stage" | "company" | "project"
>([
  ["owner_id", "member"],
  ["assignee_id", "member"],
  ["stage_id", "stage"],
  ["company_id", "company"],
  ["parent_company_id", "company"],
  ["partner_company_id", "company"],
  ["project_id", "project"],
]);

/**
 * Names the ids one history field holds. An id it cannot name, or a field that
 * holds no id, answers undefined and the value stays as stored.
 */
export function useHistoryNameOf(
  field: string,
  values: readonly (string | null | undefined)[],
): (id: string) => string | undefined {
  const { locale } = useLocale();
  const kind = ID_FIELD_KINDS.get(field);
  const ids = kind
    ? values.filter((value): value is string => !!value && isUuid(value))
    : [];
  const members = useMemberNames(kind === "member" ? ids : []);
  const records = useQueries({
    queries:
      kind === "company" || kind === "project"
        ? ids.map((id) => entityNameObserverQuery(kind, id))
        : [],
  });
  const stages = useQuery({
    ...pipelinesWithArchivedOptions,
    enabled: kind === "stage" && ids.length > 0,
    select: (pipelines) =>
      new Map(
        pipelines.flatMap((pipeline) =>
          (pipeline.stages ?? []).map(
            (stage) =>
              [
                stage.id,
                translatedLabel(stage.name, stage.name_translations, locale),
              ] as const,
          ),
        ),
      ),
  });
  const named = new Map(members);
  records.forEach((read, index) => {
    const id = ids[index];
    if (id !== undefined && read.data) {
      named.set(id, read.data);
    }
  });
  return (id) => named.get(id) ?? stages.data?.get(id);
}
