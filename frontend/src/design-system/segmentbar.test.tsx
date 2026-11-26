/** @vitest-environment happy-dom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { SegmentBar, SegmentSwatch } from "./segmentbar";

afterEach(cleanup);

describe("SegmentBar lays disjoint parts end to end", () => {
  const PARTS = [
    { key: "won", label: "Already won", value: 25, amount: "€25" },
    { key: "evidence", label: "Evidence", value: 50, amount: "€50" },
  ] as const;

  function widths(container: HTMLElement): string[] {
    return [...container.querySelectorAll<HTMLElement>(".segbar-part")].map(
      (part) => part.style.width,
    );
  }

  it("scales the parts to their own total when no mark reaches past it", () => {
    const { container } = render(<SegmentBar label="Period" parts={PARTS} />);
    expect(widths(container)).toEqual([
      "33.33333333333333%",
      "66.66666666666666%",
    ]);
  });

  // A target past the parts leaves recessed track between them and the mark,
  // which is the gap a reader is looking for.
  it("stretches the scale to a mark beyond the parts", () => {
    const { container } = render(
      <SegmentBar
        label="Period"
        parts={PARTS}
        marker={{ key: "call", label: "Call", value: 100, amount: "€100" }}
      />,
    );
    expect(widths(container)).toEqual(["25%", "50%"]);
    const mark = container.querySelector<HTMLElement>(".segbar-marker");
    expect(mark?.style.left).toBe("100%");
  });

  // The track is hidden from assistive tech, so a screen-reader user meets
  // the figures in the legend alone. Every part and the mark must be in it.
  it("names every part and the mark with its figure in the legend", () => {
    render(
      <SegmentBar
        label="Period"
        parts={PARTS}
        marker={{ key: "call", label: "Call", value: 100, amount: "€100" }}
      />,
    );
    const legend = screen.getByRole("list", { name: "Period" });
    expect(
      [...legend.querySelectorAll("li")].map((item) => item.textContent),
    ).toEqual(["Already won€25", "Evidence€50", "Call€100"]);
  });

  it("draws an empty track when nothing has been measured", () => {
    const { container } = render(
      <SegmentBar
        label="Period"
        parts={[{ key: "won", label: "Won", value: 0, amount: "€0" }]}
      />,
    );
    expect(widths(container)).toEqual(["0%"]);
  });
});

describe("SegmentBar beside readings that name its parts", () => {
  it("keeps the legend for a screen reader alone when it is hidden", () => {
    const { container } = render(
      <SegmentBar
        label="Period"
        legend="hidden"
        parts={[{ key: "won", label: "Won", value: 25, amount: "€25" }]}
      />,
    );
    const legend = screen.getByRole("list", { name: "Period" });
    expect(legend.className).toContain("sr-only");
    expect(container.querySelector(".segbar-part")).not.toBeNull();
  });

  it("draws a swatch in the colour of the part it stands for", () => {
    const { container } = render(<SegmentSwatch step="ahead" />);
    expect(
      container.querySelector(".segbar-swatch.segbar-ahead"),
    ).not.toBeNull();
  });
});
