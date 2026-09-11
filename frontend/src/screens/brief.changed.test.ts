import { describe, expect, it } from "vitest";
import { changedSinceBrief } from "./brief.changed";
import type { Worklist, WorklistItem } from "./worklist.queries";

// What has happened since the night looked.
//
// The cases are all about the difference between three states the wire keeps
// apart and a careless client would not: changed, not changed, and no run to
// compare against. The helper is pure, so they are asked of it directly — the
// count is rendered by the Today panel's own head, and a render test here would
// be asserting that panel's copy from the wrong file.

function item(over: Partial<WorklistItem> = {}): WorklistItem {
  return {
    id: "i1",
    source: "waiting_customer",
    category: "customer_waiting",
    title: "Aster Handel",
    because: [],
    actions: ["open"],
    ...over,
  } as unknown as WorklistItem;
}

function day(queue: WorklistItem[]): Worklist {
  return { queue, as_of: "2026-09-03T06:42:00Z" } as unknown as Worklist;
}

describe("changedSinceBrief", () => {
  it("counts the rows the server marked as new, and only those", () => {
    const changed = changedSinceBrief(
      day([
        item({ id: "a", changed_since_brief: true }),
        item({ id: "b", changed_since_brief: true }),
        item({ id: "c", changed_since_brief: false }),
      ]),
    );

    expect(changed?.count).toBe(2);
  });

  // ABSENT IS NOT FALSE. A row carries no flag at all when there was no run to
  // compare against, and a morning with no night is not a morning where nothing
  // changed. Counting an absent flag as "unchanged" is harmless; counting it as
  // changed would report movement nobody observed.
  it("ignores a row the night never saw, and a row it saw standing still", () => {
    expect(
      changedSinceBrief(
        day([item({ id: "a" }), item({ id: "b", changed_since_brief: false })]),
      ),
    ).toBeUndefined();
  });

  // The DECK above answers approvals, and it draws them as cards at the same
  // moment. Counting them here reported the same work twice on one page — which
  // is why this reads `waitingRows` rather than the raw queue.
  it("leaves out the approvals the decisions deck already answers", () => {
    const changed = changedSinceBrief(
      day([
        item({ id: "a", source: "approval", changed_since_brief: true }),
        item({ id: "b", changed_since_brief: true }),
      ]),
    );

    expect(changed?.count).toBe(1);
  });

  // Nothing moved, so there is nothing to say. A head that reported "0 changed"
  // every morning would teach a reader to stop reading it.
  it("says nothing at all on a morning where nothing moved", () => {
    expect(changedSinceBrief(day([]))).toBeUndefined();
  });

  // A payload with no queue at all answers nothing rather than throwing: a page
  // that throws is a worse answer than one that draws nothing.
  it("answers nothing rather than throwing on a payload with no queue", () => {
    expect(changedSinceBrief({} as unknown as Worklist)).toBeUndefined();
    expect(changedSinceBrief(undefined)).toBeUndefined();
  });

  // The door carries the SAME narrowing the count was taken over. A bare
  // `#/worklist` named three rows and opened a queue of forty, so the count and
  // its door shared nothing at all — which is the whole reason the server grew
  // this filter.
  it("points at exactly the rows it counted", () => {
    const changed = changedSinceBrief(
      day([item({ id: "a", changed_since_brief: true })]),
    );

    expect(changed?.href).toBe("#/worklist?filter=changed_since_brief");
  });
});
