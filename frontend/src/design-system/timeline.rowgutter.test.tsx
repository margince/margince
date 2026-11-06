/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render as rtlRender, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { browserZone } from "../format/timezone";
import { LocaleProvider } from "../i18n";
import { GroupedTimelineList, TimelineList } from "./timeline";
import type { TimelineEntry, TimelineGroup } from "./timelineentry";

afterEach(cleanup);

const render = (ui: ReactNode) =>
  rtlRender(<LocaleProvider initial="en">{ui}</LocaleProvider>);

function note(id: string, atIso: string): TimelineEntry {
  return {
    id,
    kind: "note",
    title: `Note ${id}`,
    atIso,
    atPrecision: "instant",
    provenance: { kind: "human", self: false },
  };
}

function single(entry: TimelineEntry): TimelineGroup {
  return { id: entry.id, kind: "single", entries: [entry], partial: false };
}

// The day each row's gutter shows on screen; a hidden day reads as "".
function shownDays(container: HTMLElement): string[] {
  return [...container.querySelectorAll(".tl-when")].map((when) =>
    [...when.childNodes]
      .filter((node) => node.nodeType === Node.TEXT_NODE)
      .map((node) => node.textContent)
      .join(""),
  );
}

describe("the date gutter", () => {
  it("prints a day once, on the first of its rows", () => {
    const { container } = render(
      <TimelineList
        zone={browserZone()}
        entries={[
          note("c", "2026-07-03T13:00:00Z"),
          note("b", "2026-07-03T11:00:00Z"),
          note("a", "2026-07-01T11:00:00Z"),
        ]}
      />,
    );
    const [first, second, third] = shownDays(container);
    expect(first).not.toBe("");
    expect(second).toBe("");
    expect(third).not.toBe("");
    expect(third).not.toBe(first);
  });

  it("keeps the repeated day for a screen reader", () => {
    const { container } = render(
      <GroupedTimelineList
        zone={browserZone()}
        groups={[
          single(note("b", "2026-07-03T13:00:00Z")),
          single(note("a", "2026-07-03T11:00:00Z")),
        ]}
      />,
    );
    const whens = container.querySelectorAll(".tl-when");
    expect(whens[1]?.querySelector(".sr-only")?.textContent).toBe(
      shownDays(container)[0],
    );
  });
});

describe("an email row", () => {
  it("says whose move it is once, and prints its time once", () => {
    const at = "2026-07-03T10:00:00Z";
    const { container } = render(
      <GroupedTimelineList
        zone={browserZone()}
        groups={[
          single({
            id: "m1",
            kind: "email",
            title: "Re: Renewal",
            atIso: at,
            atPrecision: "instant",
            provenance: { kind: "connector", connector: "gmail" },
            direction: "inbound",
            emailSummary: {
              activity_id: "m1",
              occurred_at: at,
              occurred_precision: "instant",
              direction: "inbound",
              counterparty: "Ida Keller",
              attachment_count: 0,
              move: "needs_reply",
              display_status: "team",
              version: 1,
            },
          }),
        ]}
      />,
    );
    expect(screen.getAllByText("Needs reply")).toHaveLength(1);
    expect(screen.getByText("Needs reply").closest(".badge")).not.toBeNull();
    expect(container.querySelector(".emailentry__when")).toBeNull();
    expect(container.querySelectorAll(".tl-when-time")).toHaveLength(1);
  });
});
