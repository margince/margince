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

/**
 * What a typed date means as a stored one: the value, and how precisely it was
 * given.
 *
 * A reader who knows the month and not the day types `2024-05`, and this stores
 * the first of that month WITH the precision that says so — a date rendered
 * back as "1 May" when somebody only ever claimed "May" is the product
 * inventing a fact. Empty is empty: an unknown start is a real answer and not a
 * guess at today.
 *
 * Shared by both forms that take one. The edit form and the add form send the
 * same two columns, and a second spelling of this rule would be two answers to
 * "what does a month-long date mean".
 */
export function datePatch(value: string): {
  date?: string;
  precision?: "month" | "day";
} {
  if (!value) return {};
  return value.length === 7
    ? { date: `${value}-01`, precision: "month" }
    : { date: value, precision: "day" };
}

/**
 * Whether a typed date is one this product can send: empty, a month, or a day.
 *
 * The wire declares `date`, so `not-a-date` is a request the contract refuses —
 * and `datePatch` would append `-01` to any seven characters, turning a typo
 * into a plausible-looking day. Both forms that take a date check it with this,
 * because a rule about what a date IS cannot have one spelling in the form that
 * corrects one and another in the form that creates one.
 */
export function validDateEntry(value: string): boolean {
  return value === "" || /^\d{4}-\d{2}(-\d{2})?$/.test(value);
}
