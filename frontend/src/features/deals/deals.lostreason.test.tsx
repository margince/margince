/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { en } from "../../i18n/en";
import { DealDetails } from "./deals.details";
import { deal, render, stages } from "./deals.testkit";

// A lost deal's own pane says why it was lost.
//
// The row is drawn by ClosingRow, reached as the pane's extraRows. Both halves
// were covered apart: ClosingRow was rendered with its own object, and the
// pane was rendered with open deals. Nothing joined them, so a reason that
// stopped arriving would have failed no test.
describe("a lost deal's details pane", () => {
  it("says why the deal was lost", () => {
    render(
      <DealDetails
        deal={deal({
          status: "lost",
          lost_reason: "ZZ went with a competitor on price",
          closed_at: "2026-10-27T12:00:00Z",
        })}
        meId="me"
        stages={stages}
      />,
    );
    expect(screen.getByText(en["deals.lostReason"])).toBeInTheDocument();
    expect(
      screen.getByText("ZZ went with a competitor on price"),
    ).toBeInTheDocument();
  });

  it("asks nothing about a loss on a deal that is still open", () => {
    render(
      <DealDetails
        deal={deal({ status: "open", lost_reason: "stale" })}
        meId="me"
        stages={stages}
      />,
    );
    expect(screen.queryByText(en["deals.lostReason"])).not.toBeInTheDocument();
  });
});
