// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { cappedCountLabel } from "./cappedcount";

describe("a count read off one page", () => {
  it("prints flat when the page held everything", () => {
    expect(cappedCountLabel({ seen: 7, more: false }, "en")).toBe("7");
  });

  it("says it is a floor when there was another page behind it", () => {
    expect(cappedCountLabel({ seen: 50, more: true }, "en")).toBe("50+");
  });

  it("groups its digits the way the reader's locale groups them", () => {
    expect(cappedCountLabel({ seen: 1234, more: true }, "de")).toBe("1.234+");
  });

  // Zero is a real reading and never a cap: a page that came back empty had no
  // page behind it, so nothing here may turn "none" into "none or more".
  it("prints an empty page as none", () => {
    expect(cappedCountLabel({ seen: 0, more: false }, "en")).toBe("0");
  });
});
