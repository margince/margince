// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { acceptsRedirectURI } from "./agent-client-redirect";

describe("the server's redirect URI corpus", () => {
  const corpus: { note: string; uri: string; valid: boolean }[] = JSON.parse(
    readFileSync(
      new URL(
        "../../../../backend/internal/modules/identity/testdata/redirecturi.json",
        import.meta.url,
      ),
      "utf8",
    ),
  );

  it("reads the full corpus", () => {
    expect(corpus.length).toBeGreaterThanOrEqual(50);
  });

  it.each(corpus)("answers $note as the server does", (entry) => {
    expect(acceptsRedirectURI(entry.uri)).toBe(entry.valid);
  });
});
