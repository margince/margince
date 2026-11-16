// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { derivedSource } from "./evidencesource";

const machine = "system:transcript-reader";

describe("a profile field's source mark", () => {
  it("names the meeting a value was read from by its date", () => {
    const source = derivedSource(
      {
        captured_by: machine,
        source: "meeting",
        source_occurred_at: "2026-11-12T09:00:00Z",
        evidence_snippet: "",
        confidence: 0.8,
      },
      "en",
      "UTC",
    );
    expect(source?.origin).toMatch(/^From a meeting on .*12/);
    expect(source?.snippet).toBe("");
  });

  it("names no meeting for a value read from a website", () => {
    const source = derivedSource(
      {
        captured_by: machine,
        source: "site_read",
        evidence_snippet: "We serve banks.",
        source_url: "https://brandt.example/about",
      },
      "en",
      "UTC",
    );
    expect(source?.origin).toBeUndefined();
    expect(source?.sourceUrl).toBe("https://brandt.example/about");
  });
});
