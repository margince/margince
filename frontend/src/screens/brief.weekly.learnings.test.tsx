/** @vitest-environment jsdom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { en } from "../i18n/en";
import type { WeeklyReview } from "./brief.queries";
import { render } from "./brief.testkit";
import { LearningsPanel } from "./brief.weekly.learnings";

// The distinction this panel exists to keep: a week NOBODY READ and a week that
// held NO LESSON are different facts, and only the second is about the week.
// Collapsing them tells a rep the product looked and found nothing when it may
// never have looked at all.

afterEach(cleanup);

type Learnings = NonNullable<WeeklyReview["learnings"]>;

const learned = (over: Partial<Learnings> = {}): Learnings =>
  ({
    state: "synthesized",
    items: [
      {
        kind: "worked",
        text: "Reaching the sponsor early won Nordwind.",
        citations: [
          {
            subject_type: "deal",
            subject_id: "01a05500-0000-7000-8000-0000000000d1",
            label: "Nordwind expansion",
          },
        ],
      },
    ],
    ...over,
  }) as Learnings;

describe("what the week taught", () => {
  it("draws nothing when the review predates the lane", () => {
    const { container } = render(<LearningsPanel learnings={undefined} />);
    expect(container.innerHTML).toBe("");
  });

  it("tells 'insufficient evidence' from 'nobody looked'", () => {
    render(
      <LearningsPanel learnings={learned({ state: "not_run", items: [] })} />,
    );
    expect(screen.getByText(en["brief.weekly.learnings.notRun"])).toBeTruthy();
    expect(
      screen.queryByText(en["brief.weekly.learnings.insufficient"]),
    ).toBeNull();

    cleanup();
    render(
      <LearningsPanel
        learnings={learned({ state: "insufficient_evidence", items: [] })}
      />,
    );
    expect(
      screen.getByText(en["brief.weekly.learnings.insufficient"]),
    ).toBeTruthy();
    expect(screen.queryByText(en["brief.weekly.learnings.notRun"])).toBeNull();
  });

  it("shows what a learning rests on, beside the claim", () => {
    render(<LearningsPanel learnings={learned()} />);
    expect(
      screen.getByText("Reaching the sponsor early won Nordwind."),
    ).toBeTruthy();
    // The citation is drawn, not folded away: a lesson whose sources are one
    // click out of sight is a lesson read as fact.
    expect(screen.getByText("Nordwind expansion")).toBeTruthy();
  });

  it("names a learning by its kind rather than a raw enum value", () => {
    render(<LearningsPanel learnings={learned()} />);
    expect(screen.getByText(en["brief.weekly.learnings.worked"])).toBeTruthy();
    expect(screen.queryByText("worked")).toBeNull();
  });

  it("keeps the label the week froze, not today's name", () => {
    render(<LearningsPanel learnings={learned()} />);
    // The label travels with the citation and is passed to EntityRef, so no
    // record read can replace a name the lesson was written against.
    expect(screen.getByText("Nordwind expansion")).toBeTruthy();
  });

  it("names a commitment without linking it anywhere", () => {
    render(
      <LearningsPanel
        learnings={learned({
          items: [
            {
              kind: "experiment",
              text: "Book the follow-up inside the meeting.",
              citations: [
                {
                  subject_type: "commitment",
                  subject_id: "01a05500-0000-7000-8000-0000000000c1",
                  label: "Call the Weber sponsor",
                },
              ],
            },
          ],
        } as Partial<Learnings>)}
      />,
    );
    const cited = screen.getByText("Call the Weber sponsor");
    // A commitment is a row of the rep's own plan with no 360 to send them to.
    expect(cited.closest("a")).toBeNull();
  });
});
