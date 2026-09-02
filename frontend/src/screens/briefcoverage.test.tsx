/** @vitest-environment jsdom */
import { cleanup, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { BriefCoverage } from "./briefcoverage";
import { render } from "./home.testkit";
import type { Worklist } from "./worklist.queries";

// What the page is NOT showing, per source.
//
// A short day has two very different causes — a source that was withheld, and
// a source that simply had nothing — and a reader cannot act on the first
// without being told. `Worklist.reach` carried the answer and nothing rendered
// it until now.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const empty: Worklist = {
  as_of: "2026-06-10T06:00:00Z",
  scope: "mine",
  scope_options: ["mine"],
  queue: [],
  counts: [],
  reach: [],
  sources_unavailable: [],
  summary: { total: 0, urgent: 0 },
} as unknown as Worklist;

describe("the coverage disclosure", () => {
  // A day where every source answered has nothing to disclose. A panel that
  // said "all sources complete" every morning would teach a reader to stop
  // reading it, and then it would not be read on the morning it mattered.
  it("says nothing when there is nothing to say", () => {
    const { container } = render(<BriefCoverage day={empty} />);

    expect(container.querySelector(".brief-coverage")).toBeNull();
  });

  // A refusal is a fact about the reader's day, not a detail to expand: they
  // are being shown less than the product knows, and no amount of scrolling
  // will reveal it.
  it("names a withheld source without asking the reader to expand anything", () => {
    render(
      <BriefCoverage
        day={{
          ...empty,
          sources_unavailable: [{ source: "task", reason: "withheld" }],
        }}
      />,
    );

    expect(
      screen.getByText(
        en["worklist.source.withheld"].replace(
          "{source}",
          en["worklist.untitled.task"],
        ),
      ),
    ).toBeTruthy();
  });

  // A bounded source IS a detail: the page read it and there is simply more
  // behind it, which is a different fact from not having read it at all.
  it("puts a bounded source behind the disclosure, with its figures", async () => {
    render(
      <BriefCoverage
        day={{
          ...empty,
          reach: [
            {
              source: "task",
              considered: 200,
              shown: 25,
              more_available: true,
            },
            // Read to the end: nothing more behind it, so listing it would
            // bury the one that matters.
            {
              source: "meeting",
              considered: 4,
              shown: 4,
              more_available: false,
            },
          ],
        }}
      />,
    );

    await userEvent
      .setup()
      .click(screen.getByText(en["brief.coverage.summary"]));

    expect(screen.getByText(en["worklist.untitled.task"])).toBeTruthy();
    expect(screen.queryByText(en["worklist.untitled.meeting"])).toBeNull();
  });
});
