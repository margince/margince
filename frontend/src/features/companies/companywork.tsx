// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Whether the account has work in flight. That question decides whether the
// growth-fit panel stands in for the account's own work reading. It is asked
// of both the deals and the projects halves of the 360.

import type { components } from "../../api/schema";
import { omitted } from "../../design-system/surfacestate";

type Company360 = components["schemas"]["Company360"];
type WorkProject = components["schemas"]["Company360Project"];

/**
 * hasWorkInFlight is whether the account has anything moving, which is what
 * decides between the account's own reading and the growth-fit panel in the
 * same slot.
 *
 * A WITHHELD half counts as work: a reader who may not see the projects has
 * not been told this account has none, and swapping in the fit panel would
 * tell them exactly that.
 */
export function hasWorkInFlight(view?: Company360): boolean {
  if (!view) {
    return false;
  }
  const withheld = omitted(view, "deals") || omitted(view, "projects");
  return (
    withheld ||
    (view.deals?.data.length ?? 0) > 0 ||
    liveWork(view.projects).length > 0
  );
}

// The projects the account is about: the ones still in motion. A closed
// project is history, and the same filter the project picker applies: one
// answer to "live", so hasWorkInFlight and the picker cannot disagree.
function liveWork(projects?: readonly WorkProject[]): readonly WorkProject[] {
  return (projects ?? []).filter((project) => project.phase !== "closed");
}
