/** @vitest-environment jsdom */
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";

import type { TimelineEntry, TimelineGroup } from "../design-system/composed";
import { LocaleProvider } from "../i18n";
import { ConversationList } from "./recordconversations";

// What this cut is FOR, as assertions: only the exchanges a reader can answer
// — email and message — become rows here, each one carrying whose move it is,
// and expanding a row hands the reader the same TimelineRow the chronicle
// renders rather than a second telling of it.

function entry(
  kind: TimelineEntry["kind"],
  overrides: Partial<TimelineEntry> = {},
): TimelineEntry {
  return {
    id: overrides.id ?? `${kind}-1`,
    kind,
    title: overrides.title ?? kind,
    atIso: overrides.atIso ?? "2026-07-01T10:00:00Z",
    provenance: { kind: "human", self: false },
    ...overrides,
  };
}

function group(
  id: string,
  entries: TimelineEntry[],
  partial = false,
): TimelineGroup {
  return {
    id,
    kind: entries.length > 1 ? "thread" : "single",
    entries,
    partial,
  };
}

function draw(groups: readonly TimelineGroup[]) {
  rtlRender(
    <LocaleProvider initial="en">
      <ConversationList groups={groups} zone="UTC" />
    </LocaleProvider>,
  );
}

afterEach(cleanup);

describe("which groups become conversation rows", () => {
  it("keeps only email and message threads and drops call, meeting, note and change", () => {
    draw(
      [
        entry("call", { id: "call-1", title: "Discovery call" }),
        entry("meeting", { id: "meeting-1", title: "Kickoff meeting" }),
        entry("note", { id: "note-1", title: "Rep's own note" }),
        entry("change", { id: "change-1", title: "stage" }),
        entry("email", { id: "email-1", title: "Re: Pricing" }),
        entry("message", { id: "message-1", title: "Quick check-in" }),
      ].map((one) => group(one.id, [one])),
    );

    expect(screen.getByText("Re: Pricing")).toBeTruthy();
    expect(screen.getByText("Quick check-in")).toBeTruthy();
    expect(screen.queryByText("Discovery call")).toBeNull();
    expect(screen.queryByText("Kickoff meeting")).toBeNull();
    expect(screen.queryByText("Rep's own note")).toBeNull();
    // The change row's rendered title is the field name, which the fixture
    // above also uses as the group's own title.
    expect(screen.queryByText("stage")).toBeNull();
  });
});

describe("whose move a conversation is waiting on", () => {
  it("marks the thread that ended on their word as ours to answer", () => {
    draw([
      group("inbound-1", [
        entry("email", {
          id: "in-2",
          title: "Re: Contract terms",
          atIso: "2026-07-03T10:00:00Z",
          direction: "inbound",
        }),
        entry("email", {
          id: "in-1",
          title: "Contract terms",
          atIso: "2026-07-01T10:00:00Z",
          direction: "outbound",
        }),
      ]),
    ]);

    const row = screen.getByText("Re: Contract terms").closest("li");
    expect(row).toBeTruthy();
    expect(
      row &&
        Array.from(row.querySelectorAll(".badge")).map((b) => b.textContent),
    ).toContain("Your move");
  });

  it("marks the thread that ended on our word as waiting on them", () => {
    draw([
      group("outbound-1", [
        entry("message", {
          id: "out-1",
          title: "Following up",
          direction: "outbound",
        }),
      ]),
    ]);

    const row = screen.getByText("Following up").closest("li");
    expect(row).toBeTruthy();
    expect(
      row &&
        Array.from(row.querySelectorAll(".badge")).map((b) => b.textContent),
    ).toContain("Waiting on them");
  });
});

describe("reading a conversation", () => {
  it("draws the thread open, newest first, and folds the rest behind a count", async () => {
    const user = userEvent.setup();
    draw([
      group(
        "thread-1",
        [5, 4, 3, 2, 1].map((n) =>
          entry("email", {
            id: `m${n}`,
            title: n === 1 ? "Renewal" : "Re: Renewal",
            atIso: `2026-07-0${n}T10:00:00Z`,
            direction: n % 2 === 0 ? "outbound" : "inbound",
            body: `message ${n} of the thread`,
          }),
        ),
      ),
    ]);

    // The three newest stand open without a click — the thread is what the
    // reader came to read — and the two older ones wait behind a count.
    expect(screen.getByText("message 5 of the thread")).toBeTruthy();
    expect(screen.getByText("message 3 of the thread")).toBeTruthy();
    expect(screen.queryByText("message 2 of the thread")).toBeNull();
    expect(screen.queryByText("message 1 of the thread")).toBeNull();

    await user.click(
      screen.getByRole("button", { name: "Show 2 earlier messages" }),
    );
    expect(await screen.findByText("message 1 of the thread")).toBeTruthy();

    await user.click(
      screen.getByRole("button", { name: "Hide earlier messages" }),
    );
    expect(screen.queryByText("message 1 of the thread")).toBeNull();
    expect(screen.getByText("message 3 of the thread")).toBeTruthy();
  });

  it("offers no fold on a thread short enough to stand whole", () => {
    draw([
      group("thread-1", [
        entry("email", {
          id: "newest",
          title: "Re: Renewal",
          atIso: "2026-07-03T10:00:00Z",
          direction: "inbound",
          body: "the newest word in the thread",
        }),
        entry("email", {
          id: "older",
          title: "Renewal",
          atIso: "2026-07-01T10:00:00Z",
          direction: "outbound",
          body: "the opening message",
        }),
      ]),
    ]);

    expect(screen.getByText("the newest word in the thread")).toBeTruthy();
    expect(screen.getByText("the opening message")).toBeTruthy();
    expect(screen.queryByRole("button", { name: /earlier/ })).toBeNull();
  });
});

describe("a withheld newest message", () => {
  // The row has just told this reader the message is not theirs. Claiming
  // their move on it says they owe a reply to words they are not allowed to
  // read — and the per-message move label is already suppressed for exactly
  // that reason, so leaving the thread's chip drawing made the two disagree
  // about one row, with the unsuppressed one winning on screen.
  it("claims no move on a conversation it will not show", () => {
    // The same two-message thread the case above marks "Your move", with the
    // newest message withheld — so the ONE thing varying is whether the reader
    // may read what they are being told to answer.
    draw([
      group("withheld-1", [
        entry("email", {
          id: "in-2",
          title: "Re: Contract terms",
          atIso: "2026-07-03T10:00:00Z",
          direction: "inbound",
          withheld: true,
        }),
        entry("email", {
          id: "in-1",
          title: "Contract terms",
          atIso: "2026-07-01T10:00:00Z",
          direction: "outbound",
        }),
      ]),
    ]);

    // The thread is named by the member the reader may read: the withheld
    // one's title is the kind it was, not the subject it had.
    const row = screen.getByText("Contract terms").closest("li");
    expect(row).toBeTruthy();
    expect(screen.queryByText("Re: Contract terms")).toBeNull();
    const badges =
      row &&
      Array.from(row.querySelectorAll(".badge")).map((b) => b.textContent);
    expect(badges).not.toContain("Your move");
    // And not the other verdict either: a withheld row claims no move in
    // either direction, rather than quietly reporting the opposite one.
    expect(badges).not.toContain("Waiting on them");
  });

  it("keeps the withheld member's words off the card, open as it is", () => {
    draw([
      group("thread-1", [
        entry("email", {
          id: "newest",
          title: "Re: Renewal terms",
          atIso: "2026-07-03T10:00:00Z",
          direction: "inbound",
          withheld: true,
          body: "only participants may read this line",
        }),
        entry("email", {
          id: "older",
          title: "Renewal terms",
          atIso: "2026-07-01T10:00:00Z",
          direction: "outbound",
          body: "the opening message",
        }),
      ]),
    ]);

    // The card stands, named by the member the reader may read, and the
    // withheld member keeps its place in it: the sentence where its words
    // would be, and never the words — while the sibling's body still draws.
    expect(screen.getByText("Renewal terms")).toBeTruthy();
    expect(screen.getByText("Content for participants only")).toBeTruthy();
    expect(
      screen.queryByText("only participants may read this line"),
    ).toBeNull();
    expect(screen.getByText("the opening message")).toBeTruthy();
  });

  it("says so where the subject would go when every member is withheld", () => {
    draw([
      group("thread-1", [
        entry("email", {
          id: "newest",
          title: "email",
          atIso: "2026-07-03T10:00:00Z",
          direction: "inbound",
          withheld: true,
        }),
        entry("email", {
          id: "older",
          title: "email",
          atIso: "2026-07-01T10:00:00Z",
          direction: "outbound",
          withheld: true,
        }),
      ]),
    ]);

    // Once as the thread's subject, once per member.
    expect(screen.getAllByText("Content for participants only")).toHaveLength(
      3,
    );
    expect(screen.queryByText("email")).toBeNull();
  });
});

describe("a record with no conversations", () => {
  it("says so honestly rather than drawing an empty list", () => {
    draw([group("call-1", [entry("call", { id: "call-1" })])]);

    expect(screen.getByText("No conversations with them yet.")).toBeTruthy();
  });

  it("says so for a record with no chronology at all", () => {
    draw([]);

    expect(screen.getByText("No conversations with them yet.")).toBeTruthy();
  });
});
