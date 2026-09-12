import { describe, expect, it } from "vitest";
import { changedSinceBrief } from "./brief.changed";
import type { Worklist } from "./worklist.queries";

// What has happened since the night looked.
//
// THREE OF THESE CASES USED TO LIVE HERE AND NOW LIVE IN GO, and that is the
// change rather than a loss of coverage: which rows count as new, whether an
// absent flag means "unchanged" or "there was no night", and whether a row the
// Decisions deck already draws is also news — the browser answered all three by
// walking the queue it received, which is one page of an unfiltered read. It now
// reads a figure taken over every candidate weighed, so those rules are asserted
// where they are decided (attention/linkedfilter_test.go:
// TestTheChangedCountMatchesWhatTheLaneOpens and its sibling).
//
// What is left is what this function still decides: read the figure, stay quiet
// when there is nothing to say, and point at the same lane the figure was taken
// over.

function day(changed?: number): Worklist {
  return {
    queue: [],
    as_of: "2026-09-03T06:42:00Z",
    readings:
      changed === undefined ? undefined : { changed_since_brief: changed },
  } as unknown as Worklist;
}

describe("changedSinceBrief", () => {
  it("reports the server's own count", () => {
    expect(changedSinceBrief(day(7))?.count).toBe(7);
  });

  // The number the strip prints is NOT the number of rows this page holds. A
  // reader's first page carries twenty-five, and the figure is taken over every
  // candidate the read weighed — so a morning with more news than fits still
  // says how much there is.
  it("reports a count larger than any page it could have drawn", () => {
    expect(changedSinceBrief(day(40))?.count).toBe(40);
  });

  // Nothing moved, so there is nothing to say. A head reporting "0 changed"
  // every morning would teach a reader to stop reading it.
  it("says nothing at all on a morning where nothing moved", () => {
    expect(changedSinceBrief(day(0))).toBeUndefined();
  });

  // A payload with no readings answers nothing rather than throwing, and the
  // same for no payload at all: a page that throws is a worse answer than one
  // that draws nothing.
  it("answers nothing rather than throwing on a payload it cannot read", () => {
    expect(changedSinceBrief(day())).toBeUndefined();
    expect(changedSinceBrief({} as unknown as Worklist)).toBeUndefined();
    expect(changedSinceBrief(undefined)).toBeUndefined();
  });

  // The door carries the SAME narrowing the figure was taken over. A bare
  // `#/worklist` named three rows and opened a queue of forty, so the count and
  // its door shared nothing at all — which is why the server grew this filter,
  // and why the count is now taken by running it.
  it("points at exactly the rows it counted", () => {
    expect(changedSinceBrief(day(1))?.href).toBe(
      "#/worklist?filter=changed_since_brief",
    );
  });
});
