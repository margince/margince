// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import { UTC_ZONE } from "../format/timezone";
import {
  StandingTimeline,
  type TimelineEvent,
  type TimelineMark,
} from "./standingtimeline";

afterEach(cleanup);

const events: TimelineEvent[] = [
  { key: "a", at: "2026-06-03", title: "Kickoff", kind: "done" },
  { key: "z", at: "2026-09-30", title: "Expected close", kind: "end" },
];

const mark = (
  over: Partial<TimelineMark> & Pick<TimelineMark, "key" | "at">,
): TimelineMark => ({
  precision: "instant",
  kind: "email",
  direction: "inbound",
  what: "Email",
  title: "Scope question",
  detail: "From Dana Reyes",
  when: "10 Jul",
  ...over,
});

const draw = (
  marks: TimelineMark[],
  extra: Partial<TimelineEvent>[] = [],
  record = UTC_ZONE,
) =>
  render(
    <StandingTimeline
      label="Brandt over time"
      asOf="2026-08-13T09:00:00Z"
      record={record}
      events={[
        ...events,
        ...extra.map((e, i) => ({
          key: `x${i}`,
          at: "2026-07-01",
          title: "x",
          kind: "done" as const,
          ...e,
        })),
      ]}
      marks={marks}
    />,
  );

describe("marks on the line", () => {
  it("draws a chip for one logged item and opens a card of who, what and when", async () => {
    const user = userEvent.setup();
    draw([mark({ key: "m", at: "2026-07-10" })]);

    await user.click(screen.getByRole("button", { name: "Email" }));

    const card = screen.getByRole("region", { name: "Email" });
    expect(card).toHaveTextContent("Scope question");
    expect(card).toHaveTextContent("From Dana Reyes · 10 Jul");
  });

  it("draws one chip with a count for marks a day apart, naming the span", async () => {
    const user = userEvent.setup();
    draw([
      mark({ key: "m1", at: "2026-07-10", when: "10 Jul" }),
      mark({
        key: "m2",
        at: "2026-07-11",
        when: "11 Jul",
        title: "Re: scope",
        direction: "outbound",
      }),
    ]);

    await user.click(
      screen.getByRole("button", { name: "2 entries, 10 Jul to 11 Jul" }),
    );

    expect(screen.getByRole("region")).toHaveTextContent("Re: scope");
    expect(screen.getByRole("region")).toHaveTextContent("Scope question");
  });

  it("leaves out a mark an event already stands for", () => {
    draw(
      [mark({ key: "m", at: "2026-07-10" })],
      [{ at: "2026-07-10", activityIds: ["m"] }],
    );

    expect(screen.queryByRole("button", { name: "Email" })).toBeNull();
  });

  it("keeps the mark of an event folded into a neighbour's label, which names it nowhere", () => {
    draw(
      [mark({ key: "m", at: "2026-07-10" })],
      [
        { at: "2026-07-10", activityIds: ["m"] },
        { at: "2026-07-10T12:00:00Z", title: "Call", kind: "last" },
      ],
    );

    expect(screen.getByRole("button", { name: "Email" })).toBeInTheDocument();
  });

  it("never draws a mark outside the span the events set, so marks cannot stretch the axis", () => {
    draw([mark({ key: "old", at: "2025-01-10" })]);

    expect(screen.queryByRole("button", { name: "Email" })).toBeNull();
  });

  // Read at 23:00 on 13 Aug in Kiritimati: a day entry for the 13th is stored
  // at noon UTC, later than that instant, and has still happened.
  it("draws a day entry dated today, which the owner admits as begun", () => {
    const today = "2026-08-13T12:00:00Z";
    draw(
      [mark({ key: "day", at: today, precision: "day" })],
      [],
      "Pacific/Kiritimati",
    );
    expect(screen.getByRole("button", { name: "Email" })).toBeInTheDocument();
    cleanup();
    draw([mark({ key: "later", at: today })], [], "Pacific/Kiritimati");
    expect(screen.queryByRole("button", { name: "Email" })).toBeNull();
  });

  it("keeps the chips out of the hidden canvas, where a control would be unreachable", () => {
    draw([mark({ key: "m", at: "2026-07-10" })]);

    expect(
      screen
        .getByRole("button", { name: "Email" })
        .closest("[aria-hidden='true']"),
    ).toBeNull();
  });

  it("says how many more a long cluster holds and leaves them to History", async () => {
    const user = userEvent.setup();
    draw(
      Array.from({ length: 8 }, (_, index) =>
        mark({
          key: `m${index}`,
          at: "2026-07-10",
          title: `Note ${index}`,
          kind: "note",
          direction: undefined,
        }),
      ),
    );

    await user.click(screen.getByRole("button", { name: /8 entries/ }));

    expect(screen.getByRole("region")).toHaveTextContent("2 more in History");
  });
});

describe("a date far ahead", () => {
  it("is drawn past a break that names its month and year", () => {
    render(
      <StandingTimeline
        label="Brandt over time"
        asOf="2026-08-13T09:00:00Z"
        record={UTC_ZONE}
        events={[
          { key: "a", at: "2026-07-20", title: "Kickoff", kind: "done" },
          { key: "z", at: "2029-08-29", title: "Expected close", kind: "end" },
        ]}
      />,
    );

    expect(screen.getByText("August 2029")).toBeInTheDocument();
    expect(screen.queryByText("2028")).toBeNull();
  });
});
