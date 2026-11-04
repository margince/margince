// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { UTC_ZONE } from "../format/timezone";
import {
  StandingTimeline,
  type TimelineEvent,
  type TimelineMark,
} from "./standingtimeline";
import type { TimelineHistory } from "./standingtimeline.zoombar";

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
  side: "theirs",
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
        side: "ours",
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

  it("leaves a mark older than the zoom off the line until the reader zooms out", async () => {
    const user = userEvent.setup();
    draw([mark({ key: "old", at: "2025-01-10" })]);
    expect(screen.queryByRole("button", { name: "Email" })).toBeNull();

    await user.click(screen.getByRole("button", { name: "All time" }));

    expect(screen.getByRole("button", { name: "Email" })).toBeInTheDocument();
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
          side: undefined,
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

describe("the zoom", () => {
  const older = (over: Partial<TimelineHistory>): TimelineHistory => ({
    activities: [],
    hasNextPage: true,
    isFetchingNextPage: false,
    isError: false,
    fetchNextPage: () => undefined,
    ...over,
  });
  // An event's label on the canvas, not its line in the screen reader's list.
  const TITLE = ".standing-timeline-title";
  const sameMonth: TimelineEvent[] = [
    { key: "a", at: "2026-08-03", title: "Kickoff", kind: "done" },
    { key: "b", at: "2026-08-10", title: "Last call", kind: "last" },
  ];
  const zoomed = (over: Partial<Parameters<typeof StandingTimeline>[0]> = {}) =>
    render(
      <StandingTimeline
        label="Brandt over time"
        asOf="2026-08-13T09:00:00Z"
        record={UTC_ZONE}
        events={sameMonth}
        {...over}
      />,
    );

  it("opens at a quarter, so a busy month never hides the two before it", () => {
    zoomed({ marks: [mark({ key: "june", at: "2026-06-02" })] });

    expect(screen.getByRole("button", { name: "Quarter" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.getByRole("button", { name: "Email" })).toBeInTheDocument();
  });

  it("opens wide enough for every event it names", () => {
    zoomed({
      events: [
        { key: "a", at: "2025-06-01", title: "Kickoff", kind: "done" },
        ...sameMonth,
      ],
    });

    expect(screen.getByRole("button", { name: "All time" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    expect(screen.getByText("Kickoff", { selector: TITLE })).toBeVisible();
  });

  it("on Month draws only the last month and leaves older events off", async () => {
    const user = userEvent.setup();
    zoomed({
      events: [
        { key: "a", at: "2026-05-04", title: "Kickoff", kind: "done" },
        { key: "b", at: "2026-08-10", title: "Last call", kind: "last" },
      ],
    });
    expect(screen.getByText("Kickoff", { selector: TITLE })).toBeVisible();

    await user.click(screen.getByRole("button", { name: "Month" }));

    expect(screen.queryByText("Kickoff", { selector: TITLE })).toBeNull();
    expect(screen.getByText("Last call", { selector: TITLE })).toBeVisible();
  });

  it("pages older rows in while the zoom reaches past the oldest one on hand", () => {
    const fetchNextPage = vi.fn();
    zoomed({
      marks: [mark({ key: "m", at: "2026-08-01" })],
      history: older({ fetchNextPage }),
    });

    expect(fetchNextPage).toHaveBeenCalledTimes(1);
    expect(screen.getByText("Loading earlier items…")).toBeInTheDocument();
  });

  it("asks for nothing once the rows on hand reach the zoom's start", () => {
    const fetchNextPage = vi.fn();
    zoomed({
      marks: [mark({ key: "m", at: "2026-04-01" })],
      history: older({ fetchNextPage }),
    });

    expect(fetchNextPage).not.toHaveBeenCalled();
    expect(screen.queryByText("Loading earlier items…")).toBeNull();
  });

  it("stops asking once a page fails, so a refusal is not asked again", () => {
    const fetchNextPage = vi.fn();
    zoomed({
      marks: [mark({ key: "m", at: "2026-08-01" })],
      history: older({ fetchNextPage, isError: true }),
    });

    expect(fetchNextPage).not.toHaveBeenCalled();
  });

  it("stops at 200 rows, tasks included, and leaves the rest to History", () => {
    const fetchNextPage = vi.fn();
    const tasks = Array.from({ length: 200 }, (_, index) => ({ id: index }));
    zoomed({
      marks: [mark({ key: "m", at: "2026-08-01" })],
      history: older({ fetchNextPage, activities: tasks }),
    });

    expect(fetchNextPage).not.toHaveBeenCalled();
    expect(
      screen.getByText("Earlier items are in History."),
    ).toBeInTheDocument();
  });

  it("asks for no second page while one is on its way", () => {
    const fetchNextPage = vi.fn();
    zoomed({
      marks: [mark({ key: "m", at: "2026-08-01" })],
      history: older({ fetchNextPage, isFetchingNextPage: true }),
    });

    expect(fetchNextPage).not.toHaveBeenCalled();
  });
});
