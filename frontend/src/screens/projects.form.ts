// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { CreateField } from "../components/create";
import type { CreateFieldOption } from "../components/create.fields";
import type { MessageKey } from "../i18n/en";

// The project form's vocabulary and transport, with no React in it. It holds
// what the two dialogs ask and how the answers become the request bodies.

export type Project = components["schemas"]["Project"];
export type ProjectPhase = Project["phase"];
type CreateProjectRequest = components["schemas"]["CreateProjectRequest"];
type UpdateProjectRequest = components["schemas"]["UpdateProjectRequest"];

// The four phases in the order a project walks them. Movement goes both ways
// — a closed project can be reopened into delivery — so this is the ORDER of
// the stepper, not a one-way ladder.
export const PROJECT_PHASES = [
  "initiative",
  "pursuing",
  "delivering",
  "closed",
] as const satisfies readonly ProjectPhase[];

export const PHASE_LABEL: Record<ProjectPhase, MessageKey> = {
  initiative: "project.phase.initiative",
  pursuing: "project.phase.pursuing",
  delivering: "project.phase.delivering",
  closed: "project.phase.closed",
};

export function isProjectPhase(value: string): value is ProjectPhase {
  return (PROJECT_PHASES as readonly string[]).includes(value);
}

export type ProjectCompanyOption = { id: string; display_name: string };

/**
 * The fields both dialogs share. The company is the one difference: a project
 * names its company at birth and the update body carries no
 * `company_id`, so the edit form shows the company as a fact rather
 * than as a picker.
 */
export function projectFields(
  t: (key: MessageKey) => string,
  opts: Readonly<{
    companies: readonly ProjectCompanyOption[];
    // The company this project already names, so an edit form whose
    // pickable page does not reach it still shows the one it has.
    currentCompany?: { id: string; label: string };
    // The colleagues an owner may be picked from (useRecordOwners), the
    // current owner included even when the roster walk does not carry them.
    owners: readonly CreateFieldOption[];
    me: string;
    currentOwner: string | null;
    mode: "create" | "edit";
  }>,
): CreateField[] {
  const companyOptions = opts.companies.map((company) => ({
    value: company.id,
    label: company.display_name,
  }));
  const current = opts.currentCompany;
  return [
    { key: "name", label: "project.name", required: true },
    ...(opts.mode === "create"
      ? [
          {
            key: "company_id",
            label: "project.company" as const,
            type: "select" as const,
            required: true,
            options:
              current &&
              !companyOptions.some((option) => option.value === current.id)
                ? [
                    { value: current.id, label: current.label },
                    ...companyOptions,
                  ]
                : companyOptions,
          },
        ]
      : []),
    ...ownerField(t, opts),
    { key: "description", label: "project.description", type: "textarea" },
    { key: "target_end_date", label: "project.targetEnd", type: "date" },
  ];
}

// Anyone on the roster, the reader first: a required select opens on its
// first option, so a create defaults to the creator. The server refuses an
// owner the reader may not assign work to, as it does on reassignment.
//
// Required once the project has an owner, because a blank sends owner_id null
// and the update contract reads null as "not supplied".
function ownerField(
  t: (key: MessageKey) => string,
  opts: Readonly<{
    owners: readonly CreateFieldOption[];
    me: string;
    currentOwner: string | null;
    mode: "create" | "edit";
  }>,
): CreateField[] {
  if (opts.mode === "create" && !opts.me) {
    // Without a known reader there is no default to offer, and the server
    // stamps the creator on a create that names nobody.
    return [];
  }
  // Until the roster has answered, the reader still heads the list, so the
  // create's default is never a colleague who happened to load first.
  const mine =
    opts.owners.find((option) => option.value === opts.me) ??
    (opts.me ? { value: opts.me, label: t("project.ownerMe") } : undefined);
  const others = opts.owners.filter((option) => option.value !== opts.me);
  const options = mine ? [mine, ...others] : others;
  return [
    {
      key: "owner_id",
      label: "project.owner",
      type: "select",
      required: opts.mode === "create" || opts.currentOwner !== null,
      options,
    },
  ];
}

function str(value: unknown): string {
  return typeof value === "string" ? value.trim() : "";
}

export function mapProjectCreate(
  values: Record<string, unknown>,
): CreateProjectRequest {
  return {
    name: str(values.name),
    company_id: str(values.company_id),
    owner_id: str(values.owner_id) || null,
    description: str(values.description) || null,
    target_end_date: str(values.target_end_date) || null,
    source: "manual",
  };
}

/**
 * The edit form's values as a project patch. A blank scalar clears the field
 * (explicit null), exactly as the deal patch does; the name is the one field
 * that cannot be cleared, so a blank one is left out rather than sent.
 */
export function mapProjectUpdate(
  values: Record<string, unknown>,
): UpdateProjectRequest {
  return {
    name: str(values.name) || undefined,
    owner_id: str(values.owner_id) || null,
    description: str(values.description) || null,
    target_end_date: str(values.target_end_date) || null,
  };
}

/** One project as the edit form's initial values. */
export function projectEditRecord(project: Project): Record<
  string,
  string | number | undefined
> & {
  id: string;
  version?: number;
} {
  return {
    id: project.id,
    version: project.version,
    name: project.name,
    key: project.key ?? "",
    owner_id: project.owner_id ?? "",
    description: project.description ?? "",
    target_end_date: project.target_end_date ?? "",
  };
}
