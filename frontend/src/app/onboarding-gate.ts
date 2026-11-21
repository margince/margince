// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Route, Screen } from "../lib/router";

// OAuth consent keeps its cookie-bound, single-use nonce through onboarding.
// Sign out stands in the onboarding band, so no settings page is let through.
const ONBOARDING_GATE_EXEMPT_SCREENS: ReadonlySet<Screen> = new Set([
  "onboarding",
  "oauth-consent",
]);

export function passesOnboardingGate(route: Route): boolean {
  return ONBOARDING_GATE_EXEMPT_SCREENS.has(route.screen);
}
