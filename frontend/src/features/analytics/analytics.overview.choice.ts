// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Which of Overview's two views opens, and why. No React here, so the rule is
// read and tested on its own.

import type { components } from "../../api/schema";

type Catalog = Pick<components["schemas"]["ReportingCatalog"], "presets">;
type PresetID = components["schemas"]["ReportingPreset"]["id"];
type ScopeKind =
  components["schemas"]["AnalyticsContext"]["default_scope"]["kind"];

export const OVERVIEW_VIEWS = ["sales", "prospecting", "team"] as const;
export type OverviewView = (typeof OVERVIEW_VIEWS)[number];

// The address dial that names the view, so a link opens the view it was sent from.
export const OVERVIEW_DIAL = "overview";

// The server preset each view reads.
export const OVERVIEW_PRESET: Readonly<Record<OverviewView, PresetID>> = {
  sales: "sales",
  prospecting: "sdr",
  team: "team",
};

/**
 * The views this catalog can answer, in the order the switch offers them.
 * Team prospecting is for a reader whose own lens covers colleagues: an SDR
 * reviewing their own work has My prospecting for it.
 */
export function overviewViews(
  catalog: Catalog,
  teamLens = false,
): OverviewView[] {
  return OVERVIEW_VIEWS.filter(
    (view) =>
      (view !== "team" || teamLens) &&
      catalog.presets.some((preset) => preset.id === OVERVIEW_PRESET[view]),
  );
}

export function isOverviewView(
  value: string | null | undefined,
): value is OverviewView {
  return OVERVIEW_VIEWS.some((view) => view === value);
}

// What the reader's work says about them. A role name is not one of these: an
// SDR and an AE can hold the same role, and the view is about the work.
export type WorkContext = Readonly<{
  // The workspace's reporting framework is set up for SDR teams.
  sdrFramework: boolean;
  // Whose records the reader measures when they name nobody.
  defaultScope: ScopeKind | undefined;
  // The reader may record a manager forecast for that population.
  submitsForecast: boolean;
}>;

/**
 * The view to open with. A reader whose lens covers colleagues, or who
 * records the team's forecast, reviews sales. A reader who measures their own
 * records in a workspace that runs SDR handoffs reviews their prospecting.
 * Everyone else reviews sales.
 */
export function recommendedView(
  offered: readonly OverviewView[],
  work: WorkContext,
): OverviewView | undefined {
  if (offered.length <= 1) return offered[0];
  const ownLens = work.defaultScope === "owner";
  const wanted: OverviewView =
    ownLens && work.sdrFramework && !work.submitsForecast
      ? "prospecting"
      : "sales";
  return offered.includes(wanted) ? wanted : offered[0];
}

/**
 * The view that opens: the one the address names, then the one this reader
 * chose last, then the recommended one. A shared link therefore opens what its
 * sender saw, whatever the reader chose before.
 */
export function openingView(
  offered: readonly OverviewView[],
  named: string | null | undefined,
  remembered: string | null,
  recommended: OverviewView | undefined,
): OverviewView | undefined {
  for (const candidate of [named, remembered]) {
    if (isOverviewView(candidate) && offered.includes(candidate))
      return candidate;
  }
  return recommended;
}
