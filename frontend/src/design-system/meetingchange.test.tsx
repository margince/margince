// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";

import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { CalendarChange } from "../lib/calendarchange";
import { StoryProviders } from "../testing/story-utils";
import { MeetingChange } from "./meetingchange";

const BOOKED = {
  starts_at: "2026-12-03T14:00:00Z",
  ends_at: "2026-12-03T15:00:00Z",
  starts_precision: "instant",
} as const;

function draw(change: CalendarChange) {
  return render(
    <StoryProviders>
      <MeetingChange change={change} fallbackTitle="Updated invitation" />
    </StoryProviders>,
  );
}

afterEach(cleanup);

// A date is written with its year only outside now's year, so now is fixed.
beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-11-12T12:00:00Z"));
});
afterEach(() => {
  vi.useRealTimers();
});

describe("MeetingChange", () => {
  it("strikes out the old time and place of a moved meeting and names the guest added", () => {
    const { container } = draw({
      kind: "updated",
      title: "Pricing review with Anna Weber",
      location: "Office Berlin",
      ...BOOKED,
      changed: ["time", "location"],
      before: {
        starts_at: "2026-12-02T09:00:00Z",
        ends_at: "2026-12-02T10:00:00Z",
        starts_precision: "instant",
        location: "Microsoft Teams",
      },
      guests_added: [{ email: "ben.ito@partner.example", name: "Ben Ito" }],
    });
    expect(screen.getByText("Meeting changed")).toBeInTheDocument();
    expect(screen.getByText("Microsoft Teams").tagName).toBe("S");
    expect(screen.getByText("Added Ben Ito")).toBeInTheDocument();
    expect(
      container.querySelectorAll(".meetingchange__fact--new"),
    ).toHaveLength(2);
  });

  it("draws a canceled meeting muted, with no change lines, under the subject when it has no title", () => {
    const { container } = draw({
      kind: "canceled",
      ...BOOKED,
      changed: ["time"],
      before: { starts_at: "2026-12-02T09:00:00Z" },
    });
    expect(screen.getByText("Updated invitation")).toBeInTheDocument();
    expect(screen.getByText("Canceled")).toBeInTheDocument();
    expect(container.querySelector(".meetingchange--canceled")).not.toBeNull();
    expect(container.querySelector(".meetingchange__date--off")).not.toBeNull();
    expect(container.querySelector(".meetingchange__changes")).toBeNull();
  });

  it("keeps the booked time of a proposal standing and says who proposed", () => {
    const { container } = draw({
      kind: "proposed",
      title: "Pricing review",
      starts_at: "2026-12-04T08:00:00Z",
      ends_at: "2026-12-04T09:00:00Z",
      starts_precision: "instant",
      changed: ["time"],
      before: BOOKED,
      by: { email: "anna.weber@partner.example", name: "Anna Weber" },
    });
    expect(screen.getByText("New time proposed")).toBeInTheDocument();
    expect(container.querySelector(".meetingchange__lead")).toHaveTextContent(
      /Anna Weber proposes/,
    );
    expect(container.querySelector("s")).toBeNull();
    expect(screen.getByText(/Booked for/)).toBeInTheDocument();
  });

  it("names the guest who declined, with no change lines", () => {
    const { container } = draw({
      kind: "answered",
      title: "Pricing review",
      ...BOOKED,
      changed: [],
      by: { email: "anna.weber@partner.example", name: "Anna Weber" },
      answer: "declined",
    });
    expect(screen.getByText("Declined")).toBeInTheDocument();
    expect(container.querySelector(".meetingchange__lead")).toHaveTextContent(
      "Anna Weber declined",
    );
    expect(container.querySelector(".meetingchange__changes")).toBeNull();
  });

  it("says a renamed meeting's old title and a place it no longer has", () => {
    draw({
      kind: "updated",
      title: "Partner offsite, day two",
      starts_at: "2026-11-20T00:00:00Z",
      starts_precision: "day",
      changed: ["title", "location"],
      before: { title: "Partner offsite" },
      guests_removed: [{ email: "ben.ito@partner.example" }],
    });
    expect(screen.getByText("Partner offsite").tagName).toBe("S");
    expect(screen.getByText("no place")).toBeInTheDocument();
    expect(screen.getByText(/Removed/)).toBeInTheDocument();
  });

  it("draws no day and no time for a meeting whose start is unknown", () => {
    const { container } = draw({
      kind: "updated",
      title: "Pricing review",
      changed: [],
    });
    expect(container.querySelector("time")).toBeNull();
    expect(
      container.querySelector(".meetingchange__facts"),
    ).toBeEmptyDOMElement();
  });
});
