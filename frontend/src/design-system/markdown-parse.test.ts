// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { markdownLines } from "./markdown-parse";

describe("a body's lines as a reader sees them", () => {
  it("drops the quote marks and keeps one entry per line", () => {
    expect(
      markdownLines("> Hello\n>\n> Name: Pat\n> Email: pat@example.test"),
    ).toEqual(["Hello", "Name: Pat", "Email: pat@example.test"]);
  });

  it("reads a quote nested past any real depth as text, not a crash", () => {
    const deep = `${"> ".repeat(20_000)}still here`;
    const [line] = markdownLines(deep);
    expect(line?.endsWith("still here")).toBe(true);
  });
});
