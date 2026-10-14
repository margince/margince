// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { ProgressBar } from "./progressbar";

afterEach(cleanup);

describe("ProgressBar", () => {
  it("exposes the fraction as a named progressbar with words for the value", () => {
    render(<ProgressBar fraction={0.426} label="Import" valueText="43%" />);
    const bar = screen.getByRole("progressbar", { name: "Import" });
    expect(bar).toHaveAttribute("aria-valuenow", "43");
    expect(bar).toHaveAttribute("aria-valuetext", "43%");
    expect(bar).toHaveAttribute("aria-valuemin", "0");
    expect(bar).toHaveAttribute("aria-valuemax", "100");
  });

  it("clamps a fraction outside 0..1 to the track", () => {
    render(<ProgressBar fraction={1.7} label="Import" valueText="100%" />);
    expect(screen.getByRole("progressbar")).toHaveAttribute(
      "aria-valuenow",
      "100",
    );
    cleanup();
    render(<ProgressBar fraction={-1} label="Import" valueText="0%" />);
    expect(screen.getByRole("progressbar")).toHaveAttribute(
      "aria-valuenow",
      "0",
    );
  });
});
