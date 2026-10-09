// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { isAcquisitionKind } from "./acquisitionkinds";

describe("the acquisition vocabulary", () => {
  it("tells a kind it has words for from one a newer server might send", () => {
    expect(isAcquisitionKind("mailbox_history")).toBe(true);
    expect(isAcquisitionKind("trade_fair_scan")).toBe(false);
    expect(isAcquisitionKind("")).toBe(false);
  });
});
