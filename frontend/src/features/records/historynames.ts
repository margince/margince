// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQueries, useQuery } from "@tanstack/react-query";
import { useCallback } from "react";
import type { components } from "../../api/schema";
import { entityNameObserverQuery } from "../../components/entityref.queries";
import { useLocale } from "../../i18n";
import { translatedPipeline } from "../../lib/labeltranslation";
import { useLeadDisqualifyReasons } from "../../lib/leaddisqualifyreasons";
import { useMemberNames } from "../../lib/membernames";
import {
  pipelinesOptions,
  pipelinesWithArchivedOptions,
  stageNames,
} from "../../lib/pipelines.queries";
import { isUuid } from "../../lib/uuid";

type Pipeline = components["schemas"]["Pipeline"];

// What an id stored in a history field points at, read through the reader the
// rest of the page names that kind of record with.
const ID_FIELD_KINDS = new Map<
  string,
  "member" | "stage" | "company" | "project" | "leadReason"
>([
  ["owner_id", "member"],
  ["assignee_id", "member"],
  ["stage_id", "stage"],
  ["company_id", "company"],
  ["parent_company_id", "company"],
  ["partner_company_id", "company"],
  ["project_id", "project"],
  ["disqualify_reason_id", "leadReason"],
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
  const reasons = useLeadDisqualifyReasons(kind === "leadReason");
  const nameStages = useCallback(
    (pipelines: readonly Pipeline[]) =>
      stageNames(
        pipelines.map((pipeline) => translatedPipeline(pipeline, locale)),
      ),
    [locale],
  );
  // The live pipelines the page already holds name most stages, read from the
  // cache without a fetch. Only an id they lack asks for the archived ones.
  const live = useQuery({
    ...pipelinesOptions,
    enabled: false,
    select: nameStages,
  });
  const archived = useQuery({
    ...pipelinesWithArchivedOptions,
    enabled: kind === "stage" && ids.some((id) => !live.data?.has(id)),
    select: nameStages,
  });
  const named = new Map(members);
  records.forEach((read, index) => {
    const id = ids[index];
    if (id !== undefined && read.data) {
      named.set(id, read.data);
    }
  });
  return (id) =>
    named.get(id) ??
    (kind === "stage"
      ? (live.data?.get(id) ?? archived.data?.get(id))
      : undefined) ??
    (kind === "leadReason"
      ? reasons.data?.find((reason) => reason.id === id)?.label
      : undefined);
}
