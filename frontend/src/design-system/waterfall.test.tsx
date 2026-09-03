/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { Waterfall } from "./waterfall";

afterEach(cleanup);

const opening = { label: "Opening", value: 100, amount: "€100" };
const closing = { label: "Closing", value: 130, amount: "€130" };
const steps = [
  { key: "new", label: "New", value: 50, amount: "€50" },
  { key: "lost", label: "Lost", value: -20, amount: "-€20" },
];

const WARNING = "These steps do not add up to the closing total.";

function bars(container: HTMLElement): readonly HTMLElement[] {
  return [...container.querySelectorAll<HTMLElement>(".waterfall-fill")];
}

describe("Waterfall", () => {
  // The whole claim of the picture: these named causes account for the whole
  // difference. When they do, nothing is said; the bars are the statement.
  it("says nothing when the steps reach the closing anchor", () => {
    render(
      <Waterfall
        opening={opening}
        closing={closing}
        steps={steps}
        label="Movement"
        reconciliationWarning={WARNING}
      />,
    );
    expect(screen.queryByText(WARNING)).toBeNull();
  });

  // The check runs in PRODUCTION. A dev-only assertion is compiled out of the
  // shipped build, which leaves a manager looking at bars that quietly do not
  // add up — the exact reader who cannot tell.
  it("says so on the page when the steps do not reach it", () => {
    render(
      <Waterfall
        opening={opening}
        closing={{ ...closing, value: 999 }}
        steps={steps}
        label="Movement"
        reconciliationWarning={WARNING}
      />,
    );
    expect(screen.getByText(WARNING)).toBeTruthy();
  });

  // Every bar is drawn against the largest figure in the picture. Scaled to
  // itself, a small step would draw as tall as the total it moved.
  it("draws every bar against one scale", () => {
    const { container } = render(
      <Waterfall
        opening={opening}
        closing={closing}
        steps={steps}
        label="Movement"
        reconciliationWarning={WARNING}
      />,
    );
    const [openingBar, newBar] = bars(container);
    // 100 and 50 against a scale of 130.
    expect(openingBar.style.height).toBe(`${(100 / 130) * 100}%`);
    expect(newBar.style.height).toBe(`${(50 / 130) * 100}%`);
  });

  // Direction comes from the sign, not from the caller. A caller free to
  // colour a negative step as positive could draw a loss as a gain.
  it("takes a step's direction from its own sign", () => {
    const { container } = render(
      <Waterfall
        opening={opening}
        closing={closing}
        steps={steps}
        label="Movement"
        reconciliationWarning={WARNING}
      />,
    );
    expect(container.querySelector(".waterfall-up")).toBeTruthy();
    expect(container.querySelector(".waterfall-down")).toBeTruthy();
  });

  // The bars are aria-hidden, so the table is the ONLY thing a screen reader
  // gets. Both anchors and every step have to be in it.
  it("puts both anchors and every step in the table equivalent", () => {
    render(
      <Waterfall
        opening={opening}
        closing={closing}
        steps={steps}
        label="Movement"
        reconciliationWarning={WARNING}
      />,
    );
    const table = screen.getByRole("table", { name: "Movement" });
    for (const row of [opening, closing, ...steps]) {
      expect(table.textContent).toContain(row.label);
      expect(table.textContent).toContain(row.amount);
    }
  });

  // A period where nothing moved is a real answer: two equal anchors and no
  // steps between them, which reconciles.
  it("renders a period in which nothing moved", () => {
    const { container } = render(
      <Waterfall
        opening={opening}
        closing={opening}
        steps={[]}
        label="Movement"
        reconciliationWarning={WARNING}
      />,
    );
    expect(screen.queryByText(WARNING)).toBeNull();
    expect(bars(container)).toHaveLength(2);
  });
});
