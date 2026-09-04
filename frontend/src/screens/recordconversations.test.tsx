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

describe("expanding a conversation", () => {
  it("reveals the thread's older members, hidden until then", async () => {
    const user = userEvent.setup();
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
          body: "an older message only the expanded thread carries",
        }),
      ]),
    ]);

    // The older member's own body sits inside the group only, never in the
    // row's one-line preview of the newest message.
    expect(
      screen.queryByText("an older message only the expanded thread carries"),
    ).toBeNull();

    await user.click(screen.getByRole("button", { name: "Open" }));

    expect(
      await screen.findByText(
        "an older message only the expanded thread carries",
      ),
    ).toBeTruthy();
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

    const row = screen.getByText("Re: Contract terms").closest("li");
    expect(row).toBeTruthy();
    const badges =
      row &&
      Array.from(row.querySelectorAll(".badge")).map((b) => b.textContent);
    expect(badges).not.toContain("Your move");
    // And not the other verdict either: a withheld row claims no move in
    // either direction, rather than quietly reporting the opposite one.
    expect(badges).not.toContain("Waiting on them");
  });

  it("keeps the collapsed preview off the newest message's body", async () => {
    const user = userEvent.setup();
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

    // Withheld or not, the group's own title still names the thread — the
    // fix is scoped to the body preview, not to whether the row is there.
    expect(screen.getByText("Re: Renewal terms")).toBeTruthy();
    expect(
      screen.queryByText("only participants may read this line"),
    ).toBeNull();

    // Opened, the newest member draws through the ordinary TimelineRow,
    // which is where the withheld sentence actually lives — and the member
    // row keeps the same promise the preview kept: the withheld body never
    // renders, collapsed or expanded, while the sibling's body still does.
    await user.click(screen.getByRole("button", { name: "Open" }));

    expect(
      await screen.findByText("Content for participants only"),
    ).toBeTruthy();
    expect(
      screen.queryByText("only participants may read this line"),
    ).toBeNull();
    expect(screen.getByText("the opening message")).toBeTruthy();
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
