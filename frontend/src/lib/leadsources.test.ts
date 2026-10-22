// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { sourceKeyLabel } from "./leadsources";

const t = ((key: string) => key) as Parameters<typeof sourceKeyLabel>[2];

describe("sourceKeyLabel", () => {
  it("names a shipped key stored with an underscore as the shipped label", () => {
    expect(sourceKeyLabel("web_form", undefined, t)).toBe(
      "lead.source.webform",
    );
  });

  it("reads an unlisted token as words, never as the raw token", () => {
    expect(sourceKeyLabel("trade_fair", undefined, t)).toBe("Trade fair");
  });

  it("says a lead with no source has none", () => {
    expect(sourceKeyLabel(null, undefined, t)).toBe("lead.shortfall.noSource");
  });
});
