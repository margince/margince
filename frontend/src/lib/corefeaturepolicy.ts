// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import recordFeatures from "./corefeature-records.json" with { type: "json" };
import type { Screen } from "./router";

export type CoreFeature = components["schemas"]["CoreFeature"];

// What a destination can be withheld by: an installation feature, or one screen
// alone. A screen entry is for a reader's missing grant, where Insights,
// Progress and Goals share a feature but each needs its own read.
export type Withheld = CoreFeature | `screen:${Screen}`;

// The generated lookup keeps controls on the server's RecordEnabled decision.
const RECORD_FEATURES: ReadonlyMap<string, readonly string[]> = new Map(
  Object.entries(recordFeatures),
);

const SCREEN_FEATURE: Partial<Record<Screen, CoreFeature>> = {
  partners: "companies",
  worklist: "home",
  lists: "filters",
  tags: "filters",
  offers: "deals",
  "legal-reviews": "deals",
  progress: "analytics",
  goals: "analytics",
};

export function screenEnabled(
  screen: Screen,
  disabled: readonly Withheld[],
): boolean {
  const feature = SCREEN_FEATURE[screen] ?? screen;
  return !disabled.some(
    (entry) => entry === feature || entry === `screen:${screen}`,
  );
}

export function recordEnabled(
  kind: string,
  disabled: readonly Withheld[],
): boolean {
  return !(RECORD_FEATURES.get(kind) ?? []).some((feature) =>
    disabled.some((entry) => entry === feature),
  );
}
