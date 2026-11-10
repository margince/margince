// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import recordFeatures from "./corefeature-records.json";
import type { Screen } from "./router";

export type CoreFeature = components["schemas"]["CoreFeature"];

// The generated lookup keeps controls on the server's RecordEnabled decision.
const RECORD_FEATURES: Readonly<Record<string, readonly string[]>> =
  recordFeatures;

const SCREEN_FEATURE: Partial<Record<Screen, CoreFeature>> = {
  partners: "companies",
  worklist: "home",
  lists: "filters",
  tags: "filters",
  offers: "deals",
  "legal-reviews": "deals",
};

export function screenEnabled(
  screen: Screen,
  disabled: readonly CoreFeature[],
): boolean {
  const feature = SCREEN_FEATURE[screen] ?? screen;
  return !disabled.some((entry) => entry === feature);
}

export function recordEnabled(
  kind: string,
  disabled: readonly CoreFeature[],
): boolean {
  return !(RECORD_FEATURES[kind] ?? []).some((feature) =>
    disabled.some((entry) => entry === feature),
  );
}
