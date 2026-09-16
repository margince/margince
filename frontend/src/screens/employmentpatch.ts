import { api } from "../api/client";
import type { components } from "../api/schema";
import { ifMatch } from "../api/version";
import type { useT } from "../i18n";
import { throwProblem } from "./common";
import { sameEditValue, saveIndependentEdit } from "./independentedit";

type Employment = components["schemas"]["Contact360Employment"];
type UpdateRelationshipRequest =
  components["schemas"]["UpdateRelationshipRequest"];

// One employment row's save, in its own module because BOTH the employers panel
// and the edit dialog perform it — a screen importing another screen for it is
// what this file replaces, and the two would otherwise have to agree twice on
// version skew, on which fields clear together, and on how a missing version is
// resolved.
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
