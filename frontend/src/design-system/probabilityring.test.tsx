/** @vitest-environment happy-dom */
import { cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import "@testing-library/jest-dom/vitest";
import { LocaleProvider } from "../i18n";
import { ProbabilityRing } from "./probabilityring";

afterEach(cleanup);

function ring(percent: number) {
  return render(
    <LocaleProvider initial="en">
      <ProbabilityRing percent={percent} />
    </LocaleProvider>,
  ).container;
}

describe("ProbabilityRing", () => {
  it("states the percent in words a screen reader names", () => {
    expect(ring(40).textContent).toBe("Probability: 40%");
  });

  it("fills the ring to the percent", () => {
    const fill = ring(50).querySelector(".probability-ring-fill");
    const [drawn, whole] = (fill?.getAttribute("stroke-dasharray") ?? "")
      .split(" ")
      .map(Number);
    expect(drawn).toBeCloseTo((whole ?? 0) / 2, 1);
  });

  it("draws the bare track at zero, never a dot of fill", () => {
    const container = ring(0);
    expect(container.querySelector(".probability-ring-fill")).toBeNull();
    expect(container.textContent).toBe("Probability: 0%");
  });
});
