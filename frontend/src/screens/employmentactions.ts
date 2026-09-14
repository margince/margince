// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch } from "../api/version";
import type { RecordPickerCandidate } from "../design-system/recordpicker";
import { useT } from "../i18n";
import { throwProblem } from "./common";
import { today } from "./employmentcurrency";
import { sameEditValue, saveIndependentEdit } from "./independentedit";

// The writes behind a contact's employment list — create, end, edit and
// remove an edge, and the company search the add flow picks from. The list
// itself (contactemployers.tsx) draws; this module is what it calls.

type Employment = components["schemas"]["Contact360Employment"];
type CreateRelationshipRequest =
  components["schemas"]["CreateRelationshipRequest"];
type UpdateRelationshipRequest =
  components["schemas"]["UpdateRelationshipRequest"];

export async function searchCompanyCandidates(
  q: string,
): Promise<RecordPickerCandidate[]> {
  const { data, error } = await api.GET("/companies", {
    params: { query: { q, limit: 10 } },
  });
  if (error) {
    throwProblem(error);
  }
  return data.data.map((company) => ({
    id: company.id,
    name: company.display_name,
  }));
}

// Re-read through the scoped list endpoint when a concurrent edit requires a
// comparison; the relationship contract has no single-record GET.
export async function patchEmployment(
  employment: Employment,
  contactId: string,
  body: UpdateRelationshipRequest,
  t: ReturnType<typeof useT>,
): Promise<void> {
  const read = async () => {
    const { data, error } = await api.GET("/relationships", {
      params: {
        query: {
          contact_id: contactId,
          company_id: employment.company_id,
          kind: "employment",
          limit: 200,
        },
      },
    });
    if (error) throwProblem(error);
    const row = data.data.find((row) => row.id === employment.relationship_id);
    if (!row)
      throwProblem({ detail: t("contact.rail.employmentVersionUnresolved") });
    return row;
  };
  const original = {
    ...employment,
    id: employment.relationship_id,
    started_at: employment.started_at?.slice(0, 10),
    ended_at: employment.ended_at?.slice(0, 10),
  };
  // Older snapshots may lack a version. Compare visible fields before using
  // their fresh version; never pin an unseen change as the user's baseline.
  if (original.version === undefined) {
    const fresh = await read();
    for (const key of [
      "role",
      "started_at",
      "ended_at",
      "employment_status",
    ] as const) {
      if (!sameEditValue(original[key], fresh[key]))
        throwProblem({ code: "version_skew" });
    }
    original.version = fresh.version;
  }
  await saveIndependentEdit({
    opened: { id: original.id, original },
    patch: body,
    groups: [
      ["started_at", "started_precision", "clear_started_at"],
      [
        "ended_at",
        "ended_precision",
        "clear_ended_at",
        "employment_status",
        "is_current_primary",
      ],
    ],
    read,
    write: async (patch, version) => {
      const { data, error } = await api.PATCH("/relationships/{id}", {
        params: { path: { id: original.id }, ...ifMatch(version) },
        body: patch,
      });
      if (error) throwProblem(error);
      return data;
    },
  });
}

// Refresh the record, paginated roles and brief together after any employment
// edit so the saved value and employer summary agree.
export function useEmploymentActions(contactId: string) {
  const t = useT();
  const queryClient = useQueryClient();
  const invalidate = async () => {
    await queryClient.invalidateQueries({
      queryKey: ["contact360", contactId],
    });
    await queryClient.invalidateQueries({
      queryKey: ["contactEmployments", contactId],
    });
    await queryClient.invalidateQueries({
      queryKey: ["contactBrief", contactId],
    });
  };
  const create = useMutation({
    mutationFn: async (body: CreateRelationshipRequest) => {
      const { data, error } = await api.POST("/relationships", { body });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: invalidate,
  });
  const end = useMutation({
    mutationFn: (employment: Employment) =>
      patchEmployment(employment, contactId, { ended_at: today() }, t),
    onSuccess: invalidate,
  });
  const update = useMutation({
    mutationFn: ({
      employment,
      body,
    }: {
      employment: Employment;
      body: UpdateRelationshipRequest;
    }) => patchEmployment(employment, contactId, body, t),
    onSuccess: invalidate,
  });
  const remove = useMutation({
    mutationFn: async (relationshipId: string) => {
      const { error } = await api.DELETE("/relationships/{id}", {
        params: { path: { id: relationshipId } },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: invalidate,
  });
  return { create, end, update, remove, invalidate };
}

export type EmploymentActions = ReturnType<typeof useEmploymentActions>;
