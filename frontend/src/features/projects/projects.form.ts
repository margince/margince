// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import type { CreateField } from "../../components/create";
import type { MessageKey } from "../../i18n/en";
import { searchCompanyTargets } from "../companies/companyform";
import {
  type ProjectOwnerPicker,
  projectOwnerField,
} from "./projectownerfield";

// The project form's vocabulary and transport, with no React in it: what the
// create and edit dialogs ask, and
// sees it, and how the answers become the two request bodies.

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

/**
 * The fields both dialogs share. The company is the one difference: a project
 * names its company at birth and the update body carries no
 * `company_id`, so the edit form shows the company as a fact rather
 * than as a picker.
 */
export function projectFields(
  t: (key: MessageKey) => string,
  opts: Readonly<{
    owner: ProjectOwnerPicker;
    currentOwner: string | null;
    mode: "create" | "edit";
  }>,
): CreateField[] {
  return [
    { key: "name", label: "project.name", required: true },
    ...(opts.mode === "create"
      ? [
          {
            key: "company_id",
            label: "project.company" as const,
            required: true,
            // Searched, because a workspace holds more companies than any
            // page a form could load up front.
            searchTargets: searchCompanyTargets,
          },
        ]
      : []),
    ...projectOwnerField(t, opts.owner, {
      key: "owner_id",
      currentOwner: opts.currentOwner,
      mode: opts.mode,
    }),
    { key: "description", label: "project.description", type: "textarea" },
    { key: "target_end_date", label: "project.targetEnd", type: "date" },
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
