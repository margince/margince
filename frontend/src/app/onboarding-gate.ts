// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useEffect } from "react";
import type { Route, Screen } from "../lib/router";
import { useAddressWriters } from "../lib/urlstate";

// OAuth consent keeps its cookie-bound, single-use nonce through onboarding.
// Sign out stands in the onboarding band, so no settings page is let through.
const ONBOARDING_GATE_EXEMPT_SCREENS: ReadonlySet<Screen> = new Set([
  "onboarding",
  "oauth-consent",
]);

export function passesOnboardingGate(route: Route): boolean {
  return ONBOARDING_GATE_EXEMPT_SCREENS.has(route.screen);
}

/** Holds an unfinished journey on onboarding and says whether it is held.
 *  `described` is undefined while the company probe is in flight. */
export function useOnboardingGate(
  route: Route,
  authed: boolean,
  described: boolean | undefined,
  unfinished: boolean,
): boolean {
  const gated = authed && described !== undefined && (!described || unfinished);
  // The route is a dependency so the gate holds on every navigation. Otherwise
  // a typed hash walks past onboarding.
  const { screen, id, id2 } = route;
  const { navigateReplacing } = useAddressWriters();
  useEffect(() => {
    if (gated && !passesOnboardingGate({ screen, id, id2 })) {
      // Replacing, not pushing: this is a redirect. Pushed, Back would land on
      // the refused address, and the gate would send the reader here again.
      navigateReplacing({ screen: "onboarding", id: "company" });
    }
  }, [gated, screen, id, id2, navigateReplacing]);
  return gated;
}
