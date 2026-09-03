/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { EvidenceReceipt } from "./evidencereceipt";

afterEach(cleanup);

const counts = [
  { key: "eligible", term: "Eligible deals", value: "52" },
  { key: "priced", term: "Priced", value: "40" },
] as const;

describe("EvidenceReceipt", () => {
  it("shows every count it was given", () => {
    render(<EvidenceReceipt title="Data and evidence checked" counts={counts} />);
    for (const fact of counts) {
      expect(screen.getByText(fact.term)).toBeTruthy();
      expect(screen.getByText(fact.value)).toBeTruthy();
    }
  });

  // A receipt still loading its verdict must not draw one. A badge that
  // defaulted to a neutral "ok" would state a conclusion about inputs nobody
  // has checked, which is the exact claim this panel exists to make honestly.
  it("draws no state badge until the caller has a verdict", () => {
    const { rerender } = render(
      <EvidenceReceipt title="Data and evidence checked" counts={counts} />,
    );
    expect(screen.queryByText("Ready")).toBeNull();
    rerender(
      <EvidenceReceipt
        title="Data and evidence checked"
        counts={counts}
        state={{ label: "Ready", tone: "success" }}
      />,
    );
    expect(screen.getByText("Ready")).toBeTruthy();
  });

  // Both or neither: a summary with nothing behind it promises an explanation
  // that does not exist, and a body with no summary has no name on its control.
  it("draws the calculation only when it has both a summary and a body", () => {
    const { rerender } = render(
      <EvidenceReceipt
        title="Data and evidence checked"
        counts={counts}
        calculation={<p>Sum of weighted amounts.</p>}
      />,
    );
    expect(screen.queryByText("Sum of weighted amounts.")).toBeNull();

    rerender(
      <EvidenceReceipt
        title="Data and evidence checked"
        counts={counts}
        calculationSummary="How €1.2M was calculated"
      />,
    );
    expect(screen.queryByText("How €1.2M was calculated")).toBeNull();

    rerender(
      <EvidenceReceipt
        title="Data and evidence checked"
        counts={counts}
        calculationSummary="How €1.2M was calculated"
        calculation={<p>Sum of weighted amounts.</p>}
      />,
    );
    expect(screen.getByText("How €1.2M was calculated")).toBeTruthy();
  });

  // A surface with nothing to report is not the same as a surface that was
  // never drawn, so an empty count list still renders the panel and its title.
  it("renders with no counts at all", () => {
    render(<EvidenceReceipt title="Data and evidence checked" counts={[]} />);
    expect(screen.getByText("Data and evidence checked")).toBeTruthy();
  });
});
