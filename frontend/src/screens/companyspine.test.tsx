/** @vitest-environment jsdom */
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";

import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { CompanySpine } from "./companyspine";

// The thread's one rule: it may only draw what the payload supports. Every
// stop is a record or a date, and the ONE stop with neither behind it — the
// silence — is arithmetic between two dates the payload does carry.

type View = components["schemas"]["Organization360"];

const page = { has_more: false, next_cursor: null };
const AS_OF = "2026-08-25T09:00:00Z";
const SPOKE = "2026-08-18T09:00:00Z";

function view(overrides: Record<string, unknown> = {}): View {
  return {
    as_of: AS_OF,
    organization: {
      id: "o-1",
      display_name: "Kugellager",
      captured_by: "human:u1",
      source: "manual",
      version: 1,
      created_at: "2026-06-01T08:00:00Z",
      updated_at: "2026-08-01T08:00:00Z",
    },
    sections_omitted: [],
    ...overrides,
  } as unknown as View;
}

function draw(v: View) {
  render(
    <LocaleProvider initial="en">
      <CompanySpine view={v} />
    </LocaleProvider>,
  );
}

afterEach(cleanup);

describe("the silence between the last word and today", () => {
  it("counts the days since the last conversation and says nobody replied", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        last_inbound_at: null,
        health: { last_meeting_at: SPOKE, single_threaded: true },
      }),
    );

    expect(screen.getByText("7 days")).toBeTruthy();
    expect(screen.getByText("They have never written back")).toBeTruthy();
    expect(
      screen.getByText("One contact, and no reply from them"),
    ).toBeTruthy();
  });

  it("draws no gap when they answered after we did", () => {
    // A conversation in progress is not a silence, and telling a reader to
    // chase somebody who has already written is the one thing this stop must
    // never do.
    draw(
      view({
        last_outbound_at: SPOKE,
        last_inbound_at: "2026-08-24T09:00:00Z",
        health: { last_meeting_at: SPOKE },
      }),
    );

    expect(screen.queryByText(/days/)).toBeNull();
    expect(screen.queryByText("They have never written back")).toBeNull();
  });

  it("tells a stalled relationship from one that never started", () => {
    // Both are silence; they lead to different moves. A reply that stopped is
    // a relationship to revive, and no reply at all is one to begin.
    draw(
      view({
        last_outbound_at: SPOKE,
        last_inbound_at: "2026-07-01T09:00:00Z",
        health: { last_meeting_at: SPOKE },
      }),
    );

    expect(screen.getByText("Silence since then")).toBeTruthy();
    expect(screen.queryByText("They have never written back")).toBeNull();
  });

  it("draws nothing at all on an account nobody has spoken to", () => {
    // No conversation is no thread: a silence measured from nothing would be
    // counting days since the record was created, which is not a fact about
    // the relationship.
    const { container } = render(
      <LocaleProvider initial="en">
        <CompanySpine view={view({ last_outbound_at: null })} />
      </LocaleProvider>,
    );
    expect(container.textContent).toBe("");
  });
});

describe("what the thread says is coming", () => {
  it("puts a slipped commitment on the thread as late, not as ahead", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        health: { last_meeting_at: SPOKE },
        next_steps: {
          data: [
            {
              activity_id: "s-1",
              subject: "Send the NDA",
              due_at: "2026-08-20T09:00:00Z",
              overdue: true,
            },
          ],
          page,
        },
      }),
    );

    expect(screen.getByText("Send the NDA")).toBeTruthy();
    expect(screen.getByText("Past its date")).toBeTruthy();
  });

  it("names what is riding on the close date", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        health: { last_meeting_at: SPOKE },
        state_strip: {
          account: { lifecycle: "opportunity", relationship_types: [] },
          commercial: {
            open_count: 1,
            stalled_count: 0,
            priced_count: 1,
            converted_count: 0,
            open_pipeline_minor_base: 4_800_000,
            base_currency: "EUR",
            next_close_on: "2026-10-20",
          },
        },
      }),
    );

    expect(screen.getByText("Expected close")).toBeTruthy();
    expect(screen.getByText(/riding on it/)).toBeTruthy();
  });

  it("says a pipeline is unpriced rather than printing it as worth nothing", () => {
    draw(
      view({
        last_outbound_at: SPOKE,
        health: { last_meeting_at: SPOKE },
        state_strip: {
          account: { lifecycle: "opportunity", relationship_types: [] },
          commercial: {
            open_count: 1,
            stalled_count: 0,
            priced_count: 0,
            converted_count: 0,
            next_close_on: "2026-10-20",
          },
        },
      }),
    );

    expect(screen.getByText("1 open, none priced yet")).toBeTruthy();
    expect(screen.queryByText(/€0/)).toBeNull();
  });
});
