/** @vitest-environment happy-dom */
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { FactorSources } from "./leads.score.sources";

type ScoreFactor = components["schemas"]["LeadScoreFactor"];
type ActivityReference = components["schemas"]["ActivityReference"];

function activity(over: Partial<ActivityReference> = {}): ActivityReference {
  return {
    activity_id: crypto.randomUUID(),
    kind: "email",
    subject: "Pricing follow-up",
    occurred_at: "2026-03-04T10:15:00Z",
    occurred_precision: "instant",
    content_state: "available",
    ...over,
  };
}

function show(ui: ReactNode) {
  return render(<LocaleProvider initial="en">{ui}</LocaleProvider>);
}

describe("the activities behind a score factor", () => {
  it("opens the count onto the activities it counts", async () => {
    const factor: ScoreFactor = {
      factor: "reply",
      points: 4,
      source_activity_ids: ["a", "b"],
      source_activities: [
        activity({ subject: "Pricing follow-up" }),
        activity({ subject: "Re: contract draft" }),
      ],
    };
    show(<FactorSources factor={factor} />);

    await userEvent.click(screen.getByRole("button", { name: /2 activities/ }));

    expect(await screen.findByText("Pricing follow-up")).toBeTruthy();
    expect(screen.getByText("Re: contract draft")).toBeTruthy();
  });

  // The count is the ids' length; the list is what this reader may see. A
  // card showing fewer rows than its count says so, rather than letting the
  // reader believe they are looking at all of them.
  it("says the rows are fewer than the count when the reader cannot see them all", async () => {
    const factor: ScoreFactor = {
      factor: "meeting_held",
      points: 9,
      source_activity_ids: ["a", "b", "c"],
      source_activities: [activity({ subject: "Discovery call" })],
    };
    show(<FactorSources factor={factor} />);

    await userEvent.click(screen.getByRole("button", { name: /3 activities/ }));

    expect(await screen.findByText(/showing 1 of 3/i)).toBeTruthy();
  });

  // Withheld and absent are both a null subject, told apart by content_state.
  it("says the content is withheld rather than showing an empty subject", async () => {
    const factor: ScoreFactor = {
      factor: "reply",
      points: 4,
      source_activity_ids: ["a"],
      source_activities: [
        activity({ subject: null, content_state: "withheld" }),
      ],
    };
    show(<FactorSources factor={factor} />);

    await userEvent.click(screen.getByRole("button", { name: /1 activities/ }));

    expect(
      await screen.findByText("Content for participants only"),
    ).toBeTruthy();
  });

  // Every source hidden from this reader. The server answers an empty list
  // rather than omitting the field. A count with no rows behind it must not
  // become a door that opens nothing.
  it("leaves the count as plain words when every source is hidden", () => {
    const factor: ScoreFactor = {
      factor: "reply",
      points: 4,
      source_activity_ids: ["a", "b"],
      source_activities: [],
    };
    show(<FactorSources factor={factor} />);

    expect(screen.getByText("2 activities")).toBeTruthy();
    expect(screen.queryByRole("button")).toBeNull();
  });

  // A fit factor has no sources, so there is nothing to count and nothing to
  // open.
  it("draws nothing for a factor with no sources", () => {
    const factor: ScoreFactor = { factor: "decision_maker_title", points: 12 };
    const { container } = show(<FactorSources factor={factor} />);

    expect(container.textContent).toBe("");
  });

  // A missing source_activities must not open an empty card, so the count
  // stays plain words.
  it("leaves the count as plain words against a server that names nothing", () => {
    const factor: ScoreFactor = {
      factor: "reply",
      points: 4,
      source_activity_ids: ["a", "b"],
    };
    show(<FactorSources factor={factor} />);

    expect(screen.getByText("2 activities")).toBeTruthy();
    expect(screen.queryByRole("button")).toBeNull();
  });
});
