// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Route, Screen } from "../lib/router";
import { settingsHref } from "../lib/settingsrouting";

// OAuth consent keeps its cookie-bound, single-use nonce through onboarding.
const ONBOARDING_GATE_EXEMPT_SCREENS: ReadonlySet<Screen> = new Set([
  "onboarding",
  "oauth-consent",
]);

// Settings > Account holds Sign out, so a reader mid-onboarding can always
// leave. Every other settings page stays behind the gate.
const ONBOARDING_GATE_EXEMPT_ROUTE = settingsHref("account");

export function passesOnboardingGate(route: Route): boolean {
  return (
    ONBOARDING_GATE_EXEMPT_SCREENS.has(route.screen) ||
    (route.screen === ONBOARDING_GATE_EXEMPT_ROUTE.screen &&
      route.id === ONBOARDING_GATE_EXEMPT_ROUTE.id &&
      route.id2 === undefined)
  );
}
