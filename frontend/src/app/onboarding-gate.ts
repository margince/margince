// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Route, Screen } from "../lib/router";
import { settingsHref } from "../lib/settingsrouting";

// OAuth consent keeps its cookie-bound, single-use nonce through onboarding.
const ONBOARDING_GATE_EXEMPT_SCREENS: ReadonlySet<Screen> = new Set([
  "onboarding",
  "oauth-consent",
]);

// The onboarding band links to Settings > Account, so a reader mid-setup can
// fix their language, password or name. Every other settings page waits.
const ONBOARDING_GATE_EXEMPT_ROUTE = settingsHref("account");

// Where the gate sends a held reader; the onboarding screen opens their step.
export const ONBOARDING_ENTRY: Route = { screen: "onboarding", id: "company" };

export function passesOnboardingGate(route: Route): boolean {
  return (
    ONBOARDING_GATE_EXEMPT_SCREENS.has(route.screen) ||
    (route.screen === ONBOARDING_GATE_EXEMPT_ROUTE.screen &&
      route.id === ONBOARDING_GATE_EXEMPT_ROUTE.id &&
      route.id2 === undefined)
  );
}
