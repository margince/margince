/** @vitest-environment jsdom */
import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { render } from "./brief.testkit";
import { BriefCoverage } from "./briefcoverage";
import type { Worklist } from "./worklist.queries";

// What the page is NOT showing, per source.
//
// A short day has two very different causes — a source that was withheld, and
// a source that simply had nothing — and a reader cannot act on the first
// without being told.

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

describe("the coverage line", () => {
  // A day where every source answered has nothing to disclose. A line that said
  // "every source answered" every morning would teach a reader to stop reading
  // it, and then it would not be read on the morning it mattered.
  it("says nothing when there is nothing to say", () => {
    const { container } = render(<BriefCoverage day={empty} />);

    expect(container.querySelector(".brief-coverage")).toBeNull();
  });

  // A refusal is a fact about the reader's standing, and no amount of clicking
  // will reveal what it withheld — so it is on the page, in the open.
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
        { exact: false },
      ),
    ).toBeTruthy();
  });

  // ONE LINE, and it is read rather than opened. It was a Disclosure, which
  // cost two presses and a reflow for one sentence — and it stood ABOVE the
  // figures it qualified.
  it("states a bounded source's figures with nothing to press", () => {
    const { container } = render(
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

    const line = container.querySelector(".brief-coverage");
    // WHAT THE PAGE HAS, and that more exists. `considered` is deliberately not
    // printed: it is itself a floor where the source was bounded, so putting it
    // beside `shown` invited a reader to subtract two numbers, one of which is
    // not a total.
    expect(line?.textContent).toContain(
      en["brief.coverage.bounded"]
        .replace("{source}", en["worklist.untitled.task"])
        .replace("{shown}", "25"),
    );
    expect(line?.textContent).not.toContain("200");
    expect(line?.textContent).not.toContain(en["worklist.untitled.meeting"]);
    expect(container.querySelector("button, summary")).toBeNull();
  });

  // THE CONTRADICTION THIS FIXES. `considered` is itself a floor where the
  // source was bounded, so a source whose page carries everything it counted
  // read "8 shown of at least 8 read" — a sentence that claims something is
  // held back and then accounts for all of it. The line now says what the page
  // HAS and that more exists, which is true in both cases, so there is no
  // branch left to get wrong.
  it("never says a source is short of a figure it matched exactly", () => {
    const { container } = render(
      <BriefCoverage
        day={{
          ...empty,
          reach: [
            { source: "task", considered: 8, shown: 8, more_available: true },
          ],
        }}
      />,
    );

    const said = container.querySelector(".brief-coverage")?.textContent ?? "";
    expect(said).toContain(
      en["brief.coverage.bounded"]
        .replace("{source}", en["worklist.untitled.task"])
        .replace("{shown}", "8"),
    );
    expect(said).not.toMatch(/of at least/);
  });

  // ONE LINE, and it reads as one: the lead names what the whole clause is
  // about and every source hangs off it under the same separator. Joined
  // without it the line read "read to a limit: A task at least 8", which is
  // not a sentence in any of the three languages.
  it("leads the bounded sources with what the clause is about", () => {
    const { container } = render(
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
            {
              source: "meeting",
              considered: 40,
              shown: 4,
              more_available: true,
            },
          ],
        }}
      />,
    );

    const said = container.querySelector(".brief-coverage")?.textContent ?? "";
    expect(said.startsWith(en["brief.coverage.line"].split("{")[0])).toBe(true);
    // Both sources on the one line, under the one lead.
    expect(said).toContain(en["worklist.untitled.task"]);
    expect(said).toContain(en["worklist.untitled.meeting"]);
    expect(said.match(/Read to a limit/g)).toHaveLength(1);
  });
});
