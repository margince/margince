// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { de } from "../i18n/de";
import { en } from "../i18n/en";
import { vi } from "../i18n/vi";
import { LIFECYCLE_LABELS, LIFECYCLE_OPTIONS } from "./companylifecycle";

describe("the company lifecycle", () => {
  it("offers the nine stages in wire order", () => {
    expect(LIFECYCLE_OPTIONS).toEqual([
      "unknown",
      "target",
      "prospect",
      "opportunity",
      "onboarding",
      "live",
      "customer",
      "former_customer",
      "disqualified",
    ]);
  });

  it("names onboarding and live in every language", () => {
    expect(en[LIFECYCLE_LABELS.onboarding]).toBe("Onboarding");
    expect(en[LIFECYCLE_LABELS.live]).toBe("Live");
    for (const catalog of [de, vi]) {
      expect(catalog[LIFECYCLE_LABELS.onboarding]).toBeTruthy();
      expect(catalog[LIFECYCLE_LABELS.live]).toBeTruthy();
    }
  });
});
