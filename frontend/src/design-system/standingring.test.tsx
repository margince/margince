// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { HealthRing, ScoreRing, StrengthRing } from "./standingring";

afterEach(cleanup);

describe("the health ring", () => {
  it("draws an arc for each rated dimension only, in its rating's colour", () => {
    render(
      <HealthRing
        word="At risk"
        tone="danger"
        arcs={[
          { key: "r", label: "Relationship", rating: "strong" },
          { key: "c", label: "Commercial" },
          { key: "p", label: "Payment", rating: "at_risk" },
        ]}
      />,
    );
    const arcs = [...document.querySelectorAll(".standing-ring-segment")];
    expect(arcs.map((arc) => arc.getAttribute("style"))).toEqual([
      "stroke: var(--success);",
      "stroke: var(--danger);",
    ]);
    expect(screen.getByRole("img")).toHaveAccessibleName(
      "At risk. 2 of 3 rated",
    );
  });
});

describe("the strength ring", () => {
  it("fills to the score in its band's colour", () => {
    render(
      <StrengthRing
        score={41}
        band="moderate"
        bandLabel="moderate"
        detail="Last contact 19 Oct"
        explain="Recency 80 %, frequency 40 %."
      />,
    );
    const fill = document.querySelector(".standing-ring-fill");
    expect(fill?.getAttribute("style")).toBe("stroke: var(--warning);");
    expect(screen.getByRole("img")).toHaveTextContent(
      "moderateLast contact 19 Oct",
    );
  });

  it("says no interaction happened rather than drawing a zero", () => {
    render(
      <StrengthRing
        score={0}
        band="none"
        bandLabel="none"
        detail="Last contact 19 Oct"
        explain="Recency 80 %, frequency 40 %."
      />,
    );
    expect(document.querySelector(".standing-ring-fill")).toBeNull();
    expect(screen.getByRole("img")).toHaveAccessibleName("No interactions yet");
    expect(screen.queryByText("0")).toBeNull();
  });
});

describe("the score ring", () => {
  it("draws the number over what it is out of, filled in its tone", () => {
    render(
      <ScoreRing score={82} tone="success" label="Score 82" caption="of 100" />,
    );
    const fill = document.querySelector(".standing-ring-fill");
    expect(fill?.getAttribute("style")).toBe("stroke: var(--success);");
    expect(screen.getByRole("img")).toHaveTextContent("82of 100");
  });

  it("draws bare track for a score of zero", () => {
    render(
      <ScoreRing score={0} tone="neutral" label="Score 0" caption="of 100" />,
    );
    expect(document.querySelector(".standing-ring-fill")).toBeNull();
    expect(screen.getByRole("img")).toHaveTextContent("0of 100");
  });
});
