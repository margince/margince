// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { settingsHref } from "../lib/settingsrouting";
import { passesOnboardingGate } from "./onboarding-gate";

describe("passesOnboardingGate", () => {
  it("lets onboarding and OAuth consent through", () => {
    expect(passesOnboardingGate({ screen: "onboarding", id: "voice" })).toBe(
      true,
    );
    expect(passesOnboardingGate({ screen: "oauth-consent" })).toBe(true);
  });

  it("holds every settings page, Account included", () => {
    expect(passesOnboardingGate(settingsHref("account"))).toBe(false);
    expect(passesOnboardingGate(settingsHref("notifications"))).toBe(false);
  });
});
