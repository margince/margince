/** @vitest-environment jsdom */
import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../i18n";
import {
  GroupedTimelineList,
  type TimelineEntry,
  type TimelineGroup,
} from "./composed";

// What a thread on the chronology SAYS, as assertions: the card's head names
// the kind, the count and the other side; each message says who wrote it in
// words the row can stand behind; and every row on the axis carries its time
// under its day.

afterEach(cleanup);

const render = (ui: ReactNode) =>
  rtlRender(<LocaleProvider initial="en">{ui}</LocaleProvider>);

function entry(overrides: Partial<TimelineEntry> = {}): TimelineEntry {
  return {
    id: overrides.id ?? "m1",
    kind: "email",
    title: "Re: Renewal",
    atIso: "2026-07-03T10:00:00Z",
    provenance: { kind: "connector", connector: "gmail" },
    ...overrides,
  };
}

function thread(entries: TimelineEntry[]): TimelineGroup {
  return { id: entries[0].id, kind: "thread", entries, partial: false };
}

describe("a thread's card", () => {
  it("names the kind, the count and who it was with, and seats the verbs in its head", () => {
    render(
      <GroupedTimelineList
        zone="UTC"
        groups={[
          thread([
            entry({
              id: "m3",
              direction: "inbound",
              counterparts: "Ida Keller",
              actions: <button type="button">Reply</button>,
            }),
            entry({
              id: "m2",
              atIso: "2026-07-02T10:00:00Z",
              direction: "outbound",
              counterparts: "Ida Keller",
            }),
            entry({
              id: "m1",
              atIso: "2026-07-01T10:00:00Z",
              direction: "inbound",
              counterparts: "Marc Dubois",
            }),
          ]),
        ]}
      />,
    );
    expect(screen.getByText("Thread")).toBeTruthy();
    expect(screen.getByText("3 messages")).toBeTruthy();
    // Each name once, however many messages it was on.
    expect(screen.getByText("Ida Keller, Marc Dubois")).toBeTruthy();
    // Inside the card, on its head line, rather than in the row's own
    // actions column outside it.
    expect(
      screen.getByRole("button", { name: "Reply" }).closest(".tl-thread"),
    ).toBeTruthy();
  });

  it("says who wrote each message, and claims 'you' only for the reader's own", () => {
    render(
      <GroupedTimelineList
        zone="UTC"
        groups={[
          thread([
            entry({
              id: "m3",
              direction: "inbound",
              counterparts: "Ida Keller",
              body: "their word",
            }),
            entry({
              id: "m2",
              atIso: "2026-07-02T10:00:00Z",
              direction: "outbound",
              counterparts: "Ida Keller",
              provenance: { kind: "human", self: true },
              body: "the reader's own reply",
            }),
            entry({
              id: "m1",
              atIso: "2026-07-01T10:00:00Z",
              direction: "outbound",
              counterparts: "Ida Keller",
              body: "a colleague's opener, captured by the connector",
            }),
          ]),
        ]}
      />,
    );
    const leads = Array.from(document.querySelectorAll(".tl-msg-lead")).map(
      (lead) => lead.textContent,
    );
    expect(leads).toEqual([
      "Ida Kellerwrote",
      "Yousent to Ida Keller",
      "Wesent to Ida Keller",
    ]);
    // Their word carries their face; ours carries a send mark, not a
    // monogram of "We".
    expect(screen.getByText("IK")).toBeTruthy();
    expect(document.querySelectorAll(".tl-msg-mark")).toHaveLength(2);
  });

  it("counts each person once across messages, and gives a face to one person", () => {
    render(
      <GroupedTimelineList
        zone="UTC"
        groups={[
          thread([
            entry({
              id: "m2",
              direction: "inbound",
              counterparts: "Ida Keller, Marc Dubois",
              counterpartNames: ["Ida Keller", "Marc Dubois"],
            }),
            entry({
              id: "m1",
              atIso: "2026-07-01T10:00:00Z",
              direction: "inbound",
              counterparts: "Ida Keller",
              counterpartNames: ["Ida Keller"],
            }),
          ]),
        ]}
      />,
    );
    // The head names the people, not the phrases: "Ida Keller" and
    // "Ida Keller, Marc Dubois" are two people, not three entries. Once in
    // the head, once as the two-person message's own lead.
    expect(screen.getAllByText("Ida Keller, Marc Dubois")).toHaveLength(2);
    expect(screen.queryByText(/Ida Keller, Ida Keller/)).toBeNull();
    // The face on the two-person message is the first person's, never a
    // monogram of the phrase.
    expect(screen.getAllByText("IK")).toHaveLength(2);
  });

  it("prefers the server's own counterparty to the resolved links", () => {
    render(
      <GroupedTimelineList
        zone="UTC"
        groups={[
          thread([
            entry({
              id: "m2",
              direction: "inbound",
              counterparts: "Ida Keller, Marc Dubois",
              emailSummary: {
                activity_id: "m2",
                occurred_at: "2026-07-03T10:00:00Z",
                direction: "inbound",
                counterparty: "Ida Keller +1",
                attachment_count: 0,
                move: "needs_reply",
                display_status: "team",
                version: 1,
              },
            }),
            entry({ id: "m1", atIso: "2026-07-01T10:00:00Z" }),
          ]),
        ]}
      />,
    );
    // Once in the card's head, once on the message itself.
    expect(screen.getAllByText("Ida Keller +1")).toHaveLength(2);
    expect(screen.queryByText("Ida Keller, Marc Dubois")).toBeNull();
  });

  it("opens a message into the drawer the surface mounted, through the row's own opener", async () => {
    const user = userEvent.setup();
    const opened: string[] = [];
    const summary = (id: string) => ({
      activity_id: id,
      occurred_at: "2026-07-03T10:00:00Z",
      direction: "inbound" as const,
      counterparty: "Ida Keller",
      preview: `what ${id} said`,
      attachment_count: 0,
      move: "none" as const,
      display_status: "team" as const,
      version: 1,
    });
    render(
      <GroupedTimelineList
        zone="UTC"
        groups={[
          thread([
            entry({
              id: "m2",
              emailSummary: summary("m2"),
              onOpenEmail: () => opened.push("m2"),
            }),
            entry({
              id: "m1",
              atIso: "2026-07-01T10:00:00Z",
              emailSummary: summary("m1"),
              onOpenEmail: () => opened.push("m1"),
            }),
          ]),
        ]}
      />,
    );
    await user.click(screen.getByText("what m1 said"));
    expect(opened).toEqual(["m1"]);
    expect(
      screen
        .getByRole("button", { name: /what m2 said/ })
        .getAttribute("aria-haspopup"),
    ).toBe("dialog");
  });

  it("marks a sealed message and leaves the open default unmarked", () => {
    render(
      <GroupedTimelineList
        zone="UTC"
        groups={[
          thread([
            entry({ id: "m2", direction: "inbound", audience: "participants" }),
            entry({
              id: "m1",
              atIso: "2026-07-01T10:00:00Z",
              direction: "inbound",
              audience: "workspace",
            }),
          ]),
        ]}
      />,
    );
    expect(document.querySelectorAll(".tl-msg .visibility")).toHaveLength(1);
  });
});

describe("a row's place on the axis", () => {
  it("carries the time under the day, in the record's zone", () => {
    render(
      <GroupedTimelineList
        zone="Europe/Berlin"
        groups={[
          thread([
            entry({ id: "m2", atIso: "2026-09-01T12:22:00Z" }),
            entry({ id: "m1", atIso: "2026-08-27T06:15:00Z" }),
          ]),
          {
            id: "call-1",
            kind: "single",
            partial: false,
            entries: [
              entry({
                id: "call-1",
                kind: "call",
                title: "Renewal terms",
                atIso: "2026-08-26T13:00:00Z",
              }),
            ],
          },
        ]}
      />,
    );
    const gutter = Array.from(document.querySelectorAll(".tl-when")).map(
      (when) => when.textContent,
    );
    expect(gutter).toEqual(["01/09/202614:22", "26/08/202615:00"]);
    // Inside the card each message carries day AND time: the gutter holds the
    // newest member's day, and an older member may be from another.
    expect(screen.getByText("27/08/2026 08:15")).toBeTruthy();
  });
});
