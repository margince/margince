// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

// The primitive exists so a unit can space two things without a stylesheet, so
// what these hold is that the SCALE decides the spacing and the caller only
// names a step — and that align and justify are two questions, which they were
// not for one revision: both spelled `ds-row-start`, so a row asked to align
// its items to the top silently justified them left as well.

import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { Row, Stack } from "./stack";

describe("Stack", () => {
  it("separates its children by a named step and draws nothing else", () => {
    render(
      <Stack gap="4">
        <span>first</span>
        <span>second</span>
      </Stack>,
    );
    const box = screen.getByText("first").parentElement;
    expect(box?.className).toBe("ds-stack ds-gap-4");
  });

  it("defaults to the field-to-field step, so a caller who says nothing is still on the scale", () => {
    render(
      <Stack>
        <span>only</span>
      </Stack>,
    );
    expect(screen.getByText("only").parentElement?.className).toContain(
      "ds-gap-3",
    );
  });
});

describe("Row", () => {
  it("carries alignment and justification as separate classes", () => {
    render(
      <Row align="start" justify="between">
        <span>left</span>
        <span>right</span>
      </Row>,
    );
    const box = screen.getByText("left").parentElement;
    expect(box?.className).toContain("ds-align-start");
    expect(box?.className).toContain("ds-justify-between");
  });

  it("wraps by default, because off-screen is the one place a chip must not go", () => {
    render(
      <Row>
        <span>chip</span>
      </Row>,
    );
    expect(screen.getByText("chip").parentElement?.className).toContain(
      "ds-row-wrap",
    );
  });

  it("holds a row on one line when the caller says so", () => {
    render(
      <Row wrap={false}>
        <span>chip</span>
      </Row>,
    );
    expect(screen.getByText("chip").parentElement?.className).toContain(
      "ds-row-nowrap",
    );
  });
});
