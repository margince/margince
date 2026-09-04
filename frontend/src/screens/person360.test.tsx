/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { RelationshipPulse, ThinState } from "./person360";

type Person360 = components["schemas"]["Person360"];

// The thin page names the employer and picks its ONE next step from whether it
// found one. Both answers come from the same rule the rail and the server use:
// the flag records WHICH employer, and whether that job is still theirs is
// derived from its last day. Get that wrong here and the page tells somebody
// serving notice's account manager to go and add an employer they already have.
const NOW = new Date("2026-08-18T09:00:00Z");

const person: components["schemas"]["Person"] = {
  id: "p-1",
  full_name: "Dana Buyer",
  first_name: "Dana",
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
  emails: [
    {
      id: "e-1",
      email: "dana@brandt.example",
      email_type: "work",
      is_primary: true,
      position: 0,
      source: "manual",
      captured_by: "human:u-1",
    },
  ],
};

const page = { has_more: false, next_cursor: null };

function viewWith(endedAt: string | null): Person360 {
  return {
    as_of: "2026-08-18T09:00:00Z",
    person,
    sections_omitted: [],
    employments: {
      data: [
        {
          relationship_id: "rel-1",
          organization_id: "o-1",
          organization_name: "Brandt Automotive GmbH",
          role: "Head of Fleet",
          is_current_primary: true,
          started_at: "2022-03-01T00:00:00Z",
          ended_at: endedAt,
        },
      ],
      page,
    },
  };
}

function mount(view: Person360) {
  render(
    <LocaleProvider initial="en">
      <ThinState view={view} />
    </LocaleProvider>,
  );
}

beforeEach(() => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  vi.setSystemTime(NOW);
});

afterEach(() => {
  vi.useRealTimers();
  cleanup();
});

function viewWithChanges(): Person360 {
  return {
    as_of: "2026-08-18T09:00:00Z",
    person,
    sections_omitted: [],
    last_inbound_at: "2026-08-17T10:00:00Z",
    last_outbound_at: "2026-07-07T10:00:00Z",
    relationship_changes: [
      { kind: "replied_after_gap", at: "2026-08-17T10:00:00Z", days: 41 },
      {
        kind: "warmed",
        at: "2026-08-18T09:00:00Z",
        from_bucket: "weak",
        to_bucket: "moderate",
      },
    ],
  };
}

// The pulse says what the relationship IS; the changes under it say what
// happened to it, and the pair is the point — "warm" is inferable from the two
// dates above, "they replied after 41 quiet days" is what makes those dates
// mean anything. A list that rendered nothing would leave the card asserting a
// state with no event behind it.
describe("what happened to the relationship", () => {
  it("says each change in the 360's own sentence", () => {
    render(
      <LocaleProvider initial="en">
        <RelationshipPulse view={viewWithChanges()} />
      </LocaleProvider>,
    );

    expect(screen.getByText("They replied after 41 quiet days.")).toBeTruthy();
    // A band move names BOTH bands: "the relationship moved" without saying
    // from what to what is a claim the reader has to take on trust.
    expect(
      screen.getByText("The relationship moved from weak to moderate."),
    ).toBeTruthy();
  });

  it("draws no list at all when nothing has moved", () => {
    const view = viewWithChanges();
    render(
      <LocaleProvider initial="en">
        <RelationshipPulse view={{ ...view, relationship_changes: [] }} />
      </LocaleProvider>,
    );

    expect(screen.queryByRole("list")).toBeNull();
  });
});

describe("the thin person page", () => {
  it("names the employer of somebody still in the job", () => {
    mount(viewWith(null));

    expect(screen.getByText(/Brandt Automotive GmbH/)).toBeTruthy();
    expect(screen.getByText(/Connect the mailbox/)).toBeTruthy();
  });

  it("still names it while they are serving notice", () => {
    mount(viewWith("2026-09-01"));

    expect(screen.getByText(/Brandt Automotive GmbH/)).toBeTruthy();
    // The next step is the one for a page that HAS an employer. Treating a
    // notice period as a departure would ask for an employer already on record.
    expect(screen.getByText(/Connect the mailbox/)).toBeTruthy();
  });

  it("asks for an employer once the last day has passed", () => {
    mount(viewWith("2026-08-04"));

    expect(screen.queryByText(/Brandt Automotive GmbH/)).toBeNull();
    expect(screen.getByText(/Add their employer/)).toBeTruthy();
  });
});
