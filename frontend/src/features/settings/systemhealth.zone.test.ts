// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { healthCodes, healthRecovery } from "./systemhealth.copy";
import { findingEffectKey, findingNextStepKey } from "./systemhealth.effect";
import { zoneFinding as finding } from "./systemhealth.fixtures";

describe("the unloadable-zone health finding", () => {
  it("says what falls back, how it recovers, and where the admin fixes it", () => {
    expect(healthCodes[finding.code]).toBe("health.code.zone_unresolved");
    expect(findingEffectKey(finding)).toBe("health.effect.zone");
    expect(findingNextStepKey(finding)).toBe("health.zoneNext");
    expect(healthRecovery[finding.recovery]).toBe("health.recovery.zone");
  });
});
