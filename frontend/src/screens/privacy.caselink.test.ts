/** @vitest-environment happy-dom */
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { components } from "../api/schema";
import { caseHref, caseInParams, useLinkedCase } from "./privacy.caselink";
import { rowHref } from "./worklist.copy";

// The worklist has named subject requests on their statutory clock since that
// lane shipped, and its row linked to the QUEUE. An officer following it landed
// on twenty rows with nothing saying which one they had been sent to read.
// What this file holds is that the address names the case, that the case it
// names opens, and that a case the page cannot show says so rather than leaving
// the reader on an ordinary queue.

function goTo(hash: string): void {
  globalThis.location.hash = hash;
}

beforeEach(() => goTo("#/settings/admin/privacy"));
afterEach(cleanup);

describe("the address of one case", () => {
  it("names the case the worklist row was about", () => {
    expect(caseHref("case-1")).toContain("case=case-1");
  });

  it("escapes an id rather than splicing it into the query", () => {
    expect(caseHref("a&b=c")).toContain("case=a%26b%3Dc");
  });

  it("reads back the case an address names", () => {
    expect(caseInParams(new Map([["case", "case-9"]]))).toBe("case-9");
    expect(caseInParams(new Map())).toBeUndefined();
  });
});

describe("the row the address opens", () => {
  it("opens the case the link named", () => {
    goTo("#/settings/admin/privacy?case=case-2");
    const { result } = renderHook(() =>
      useLinkedCase(["case-1", "case-2"], false),
    );
    expect(result.current.expandedId).toBe("case-2");
    expect(result.current.linked).toEqual({ kind: "shown", id: "case-2" });
  });

  it("opens nothing when the address names nothing", () => {
    const { result } = renderHook(() => useLinkedCase(["case-1"], false));
    expect(result.current.expandedId).toBeNull();
    expect(result.current.linked).toEqual({ kind: "none" });
  });

  // The failure this hook exists for. The queue pages twenty at a time, so the
  // linked case can be on none of the pages loaded — and a card that merely
  // seeded its expansion would show an ordinary queue with no sign the link had
  // named anything.
  it("says so when the linked case is on no page it has loaded", () => {
    goTo("#/settings/admin/privacy?case=case-99");
    const { result } = renderHook(() =>
      useLinkedCase(["case-1", "case-2"], false),
    );
    expect(result.current.linked).toEqual({ kind: "absent", id: "case-99" });
  });

  // Not absent, merely not here YET — the reader can press Load more, so
  // claiming the case does not exist would be a wrong answer they can disprove.
  it("waits rather than claiming absence while more pages remain", () => {
    goTo("#/settings/admin/privacy?case=case-99");
    const { result } = renderHook(() => useLinkedCase(["case-1"], true));
    expect(result.current.linked).toEqual({ kind: "loading", id: "case-99" });
  });

  // The bug the feature was for, one page further down. The queue pages twenty
  // at a time, so a link to case twenty-one put the reader on an ordinary list
  // with nothing saying their case was one page away and nothing fetching it.
  it("fetches forward until the linked case arrives", () => {
    goTo("#/settings/admin/privacy?case=case-21");
    const loadMore = vi.fn();
    const { rerender } = renderHook(
      ({ ids, more }: { ids: string[]; more: boolean }) =>
        useLinkedCase(ids, more, loadMore),
      { initialProps: { ids: ["case-1"], more: true } },
    );
    expect(loadMore).toHaveBeenCalled();
    // The page it asked for arrives and the chase stops.
    loadMore.mockClear();
    rerender({ ids: ["case-1", "case-21"], more: true });
    expect(loadMore).not.toHaveBeenCalled();
  });

  it("asks for nothing once the case is on screen", () => {
    goTo("#/settings/admin/privacy?case=case-1");
    const loadMore = vi.fn();
    renderHook(() => useLinkedCase(["case-1"], true, loadMore));
    expect(loadMore).not.toHaveBeenCalled();
  });

  // An id naming no case would otherwise walk the whole queue a page at a time
  // looking for it. The chase is bounded and then says so.
  it("stops chasing an id that names no case and says it is not here", () => {
    goTo("#/settings/admin/privacy?case=nothing");
    const loadMore = vi.fn();
    const { result, rerender } = renderHook(
      ({ ids }: { ids: string[] }) => useLinkedCase(ids, true, loadMore),
      { initialProps: { ids: ["case-1"] } },
    );
    for (let page = 2; page < 40; page += 1) {
      rerender({ ids: [`case-${page}`] });
    }
    // It really did chase before it gave up, so the bound is what stopped
    // it and not a path that never asked for a page at all.
    expect(loadMore.mock.calls.length).toBeGreaterThan(1);
    expect(loadMore.mock.calls.length).toBeLessThan(20);
    expect(result.current.linked).toEqual({ kind: "absent", id: "nothing" });
  });
});

describe("the address follows the reader", () => {
  it("names the case the reader opened by hand", () => {
    const { result } = renderHook(() => useLinkedCase(["case-1"], false));
    act(() => result.current.toggle("case-1"));
    expect(result.current.expandedId).toBe("case-1");
    expect(globalThis.location.hash).toContain("case=case-1");
  });

  it("drops the case from the address when the row is closed", () => {
    goTo("#/settings/admin/privacy?case=case-1");
    const { result } = renderHook(() => useLinkedCase(["case-1"], false));
    act(() => result.current.toggle("case-1"));
    expect(result.current.expandedId).toBeNull();
    expect(globalThis.location.hash).not.toContain("case=");
  });

  // A closed row that sprang back open on the next render would be a queue
  // fighting the reader, which is what the dismissal in the hook prevents.
  it("keeps a linked row closed once the reader closes it", () => {
    goTo("#/settings/admin/privacy?case=case-1");
    const { result, rerender } = renderHook(() =>
      useLinkedCase(["case-1"], false),
    );
    act(() => result.current.toggle("case-1"));
    rerender();
    expect(result.current.expandedId).toBeNull();
  });

  it("moves to another case the reader opens instead", () => {
    goTo("#/settings/admin/privacy?case=case-1");
    const { result } = renderHook(() =>
      useLinkedCase(["case-1", "case-2"], false),
    );
    act(() => result.current.toggle("case-2"));
    expect(result.current.expandedId).toBe("case-2");
    expect(globalThis.location.hash).toContain("case=case-2");
  });

  // Replace, not push: an officer who opened four cases still has one Back out
  // of the queue, which is the choice app/urlstate.ts makes for every dial.
  it("leaves one history entry behind however many cases are opened", () => {
    const { result } = renderHook(() =>
      useLinkedCase(["case-1", "case-2"], false),
    );
    const before = globalThis.history.length;
    act(() => result.current.toggle("case-1"));
    act(() => result.current.toggle("case-2"));
    expect(globalThis.history.length).toBe(before);
  });
});

// The half a reader actually follows. The row carried the queue's address and
// stopped one step short of the case it was naming.
describe("the worklist row that names a case", () => {
  type WorklistItem = components["schemas"]["WorklistItem"];

  // Typed, not cast. A cast compiles over a missing required field and the
  // test then asserts against a shape the wire never sends.
  function dsrRow(id: string): WorklistItem {
    return {
      id,
      source: "dsr",
      category: "system",
      level: 1,
      consequence: "legal_deadline_missed",
      title: "A subject access request is due",
      because: [],
      actions: [],
    };
  }

  it("links to the case rather than to the queue holding it", () => {
    expect(rowHref(dsrRow("case-7"))).toBe(caseHref("case-7"));
  });

  // Only the subject-request lane is answered on the queue. A notice case is
  // worked from the contact's own page, so it keeps the address it had — which
  // is the contact it names, and nothing at all when it names none.
  it("sends a notice case to its contact and not to the case queue", () => {
    const notice: WorklistItem = {
      ...dsrRow("notice-1"),
      source: "notice_case",
      subject: { type: "contact", id: "01a05500-0000-7000-8000-0000000000d1" },
    };
    const href = rowHref(notice);
    expect(href).toContain("contacts");
    expect(href).not.toContain("case=");
  });
});
