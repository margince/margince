/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { AiPending } from "./aipending";

afterEach(cleanup);

describe("AiPending", () => {
  it("is one pending region, announced once, with the wait named and shown", () => {
    render(<AiPending label="Margince is reading this account." lines={2} />);
    const region = screen.getByRole("status");
    expect(region.getAttribute("aria-busy")).toBe("true");
    // Shown as well as spoken: this is the long wait, and a mute block that
    // long reads as broken. Exactly one copy, so a screen reader hears it once.
    expect(
      screen.getAllByText("Margince is reading this account."),
    ).toHaveLength(1);
    expect(document.querySelectorAll(".pending-line")).toHaveLength(2);
  });

  it("keeps its ornament out of the accessibility tree", () => {
    render(<AiPending label="Reading." />);
    for (const ornament of document.querySelectorAll(
      ".aipending-tile, .aipending-sheen",
    )) {
      expect(ornament.getAttribute("aria-hidden")).toBe("true");
    }
  });
});
