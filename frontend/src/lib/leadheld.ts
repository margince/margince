// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { ProblemError } from "./problem";
import { problemCode, problemDetails } from "./problembody";

// What a 409 lead_holds_email says: the leads holding the key that the reader
// may open, and whether the reader may convert them by sending again.
export type LeadHeld = Readonly<{ leadIds: string[]; convertible: boolean }>;

export function leadHeldOf(error: unknown): LeadHeld | null {
  if (
    !(error instanceof ProblemError) ||
    problemCode(error.problem) !== "lead_holds_email"
  ) {
    return null;
  }
  const details = problemDetails(error.problem);
  const ids = Array.isArray(details?.lead_ids)
    ? details.lead_ids.filter((id): id is string => typeof id === "string")
    : [];
  return { leadIds: ids, convertible: details?.convertible === true };
}
