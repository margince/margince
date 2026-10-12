// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";

// The import surface's wire shapes, named once so the card and its mapping
// table cannot drift apart about what a profile or a report is.
export type ImportObject = components["schemas"]["ImportObject"];
export type ImportProfile = components["schemas"]["ImportSourceProfile"];
export type ImportColumn = components["schemas"]["ImportColumn"];
export type ImportRun = components["schemas"]["ImportRun"];
export type ImportReport = components["schemas"]["ImportRunReport"];
export type ImportUndoReport = components["schemas"]["ImportUndoReport"];
export type ImportFormat = components["schemas"]["ImportFormat"];
export type ImportFormatColumn = components["schemas"]["ImportFormatColumn"];
export type ImportPartProfile = components["schemas"]["ImportPartProfile"];
export type ImportRunPart = components["schemas"]["ImportRunPart"];
export type ImportPartReport = components["schemas"]["ImportPartReport"];
export type ImportProblemKind = components["schemas"]["ImportProblemKind"];
export type ImportRowIssue = components["schemas"]["ImportRowIssue"];

// DONT_IMPORT is the mapping table's resting choice: a column nobody has
// assigned a destination to is LEFT OUT, never guessed into a field. It is a
// sentinel for the picker only — the request carries the mapped columns and
// nothing else.
export const DONT_IMPORT = "";

// The row type a flow opens on when nothing names one.
export const UNNAMED_IMPORT_OBJECT: ImportObject = "lead";

// IMPORT_OBJECTS has one key per contract type; a type the contract adds fails
// to compile here until the screens are taught it.
const IMPORT_OBJECTS = {
  company: true,
  contact: true,
  lead: true,
} as const satisfies Record<ImportObject, true>;

export function isImportObject(value: unknown): value is ImportObject {
  return typeof value === "string" && Object.hasOwn(IMPORT_OBJECTS, value);
}
