// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { translate } from "../../i18n";
import { jobKindNameKey } from "./jobkinds";

describe("jobKindNameKey", () => {
  it("names a declared kind", () => {
    const key = jobKindNameKey("capture_backfill");
    expect(key).toBe("jobs.kind.capture_backfill");
    expect(key && translate("en", key)).toBe("Mail history import");
  });

  it("has no name for a kind this build does not declare", () => {
    expect(jobKindNameKey("ext_acme_sync")).toBeUndefined();
    expect(jobKindNameKey("retention_sweep_dispatch")).toBeUndefined();
    expect(jobKindNameKey("constructor")).toBeUndefined();
  });
});
