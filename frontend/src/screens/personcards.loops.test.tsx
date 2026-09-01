/** @vitest-environment jsdom */
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { PersonCommitmentsCard } from "./personcards";
import { installFetchStub, meRoute, StoryProviders } from "./story-utils";

type Person360 = components["schemas"]["Person360"];
type ConversationClaim = components["schemas"]["ConversationClaim"];

// The commitments card reads a promise against the clock, and it used to need a
// full elapsed DAY before it would say overdue — so for 24 hours the contact
// said "due yesterday" about the same promise the task list, the agent tool and
// the SQL all called late. These pin the boundary at the instant.

// The provenance every fixture person carries, typed from the contract rather
// than asserted: these are Person360's own person fields, so a change to any
// of them fails here instead of being widened away by the assertion that used
// to stand in for the type.
const CAPTURED: Pick<
  Person360["person"],
  "source" | "captured_by" | "created_at" | "updated_at"
> = {
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
};

// The reader every case renders as. useViewerId reads it off the shared /me
// cache, and whose task a row is decides how the row is prefixed.
const VIEWER = "00000000-0000-4000-8000-000000000001";

beforeEach(() => {
  installFetchStub({ "GET /me": meRoute({ activity: ["read"] }) });
});

// The reading moment every case below is an offset from. Fixed, because a
// suite that reads the machine's calendar changes its verdict in December.
const NOW = "2026-08-25T12:00:00Z";
const HOUR = 3_600_000;

function claim(dueAt: string | null): ConversationClaim {
  return {
    id: "c-1",
    kind: "commitment_ours",
    body: "Send the pilot quote",
    source_activity_id: "a-1",
    source_quote: "I'll send the quote on Monday.",
    status: "open",
    needs_review: false,
    due_at: dueAt,
  };
}

function viewWithDue(dueAt: string | null): Person360 {
  return {
    as_of: NOW,
    person: { id: "p-1", full_name: "Dana Buyer", ...CAPTURED },
    sections_omitted: [],
    claims: [claim(dueAt)],
  };
}

function at(offsetMs: number): string {
  return new Date(Date.parse(NOW) + offsetMs).toISOString();
}

function renderDue(dueAt: string | null) {
  render(
    <StoryProviders>
      <PersonCommitmentsCard view={viewWithDue(dueAt)} firstName="Dana" />
    </StoryProviders>,
  );
}

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("the commitments card's due status", () => {
  // The clock is injected, never the machine's: `Date.now()` inside the card is
  // what decides the verdict, so a real clock would move these cases every run.
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(NOW));
  });

  it("calls a promise overdue the moment it passes, not a day later", () => {
    // The reported case: 23 hours past due read as "due yesterday" here while
    // every other surface called it overdue.
    renderDue(at(-23 * HOUR));
    expect(screen.getByText("overdue by less than a day")).toBeTruthy();
    expect(screen.queryByText(/^due /)).toBeNull();
  });

  it("counts whole days only once a whole day has passed", () => {
    renderDue(at(-49 * HOUR));
    // Two whole days and one hour is two days late, never three.
    expect(screen.getByText("overdue 2 days")).toBeTruthy();
  });

  it("is not overdue at the instant it falls due", () => {
    // A reader told they have missed something at the instant it becomes due
    // has been told something untrue.
    renderDue(at(0));
    expect(screen.getByText("due today")).toBeTruthy();
  });

  it("keeps a promise still hours away under today, not tomorrow", () => {
    renderDue(at(11 * HOUR));
    expect(screen.getByText("due today")).toBeTruthy();
  });

  it("names tomorrow only for the day after this one", () => {
    renderDue(at(26 * HOUR));
    expect(screen.getByText("due tomorrow")).toBeTruthy();
  });

  it("falls back to the open badge when the due instant is unreadable", () => {
    // A due date nothing can parse names no deadline; "overdue NaN days" is
    // the one reading that must not reach a reader.
    renderDue("whenever");
    expect(screen.getByText("open")).toBeTruthy();
  });

  it("shows the open badge when there is no due date at all", () => {
    renderDue(null);
    expect(screen.getByText("open")).toBeTruthy();
  });
});

// A task IS a promise. The card read extracted claims alone, so a record whose
// only open promise was a task — which is what an accepted transcript proposal
// becomes — said "nothing has been promised" under a headline naming it.
describe("open tasks are commitments too", () => {
  function viewWithTask(dueAt: string | null): Person360 {
    return {
      as_of: NOW,
      person: { id: "p-1", full_name: "Dana Buyer", ...CAPTURED },
      sections_omitted: [],
      next_steps: {
        data: [
          {
            id: "t-1",
            kind: "task",
            subject: "Send the MCP whitepaper",
            occurred_at: NOW,
            is_done: false,
            due_at: dueAt,
            ...CAPTURED,
          },
        ],
        page: { has_more: false },
      },
    };
  }

  it("lists an undated task as ours and open", () => {
    render(
      <StoryProviders>
        <PersonCommitmentsCard view={viewWithTask(null)} firstName="Dana" />
      </StoryProviders>,
    );

    expect(
      screen.getAllByText(/Send the MCP whitepaper/).length,
    ).toBeGreaterThan(0);
    expect(screen.getByText("open")).toBeDefined();
    expect(screen.queryByText(/Nothing has been promised/)).toBeNull();
  });

  it("reads an overdue task against the clock, like a claim", () => {
    vi.setSystemTime(new Date(Date.parse(NOW)));
    render(
      <StoryProviders>
        <PersonCommitmentsCard
          view={viewWithTask(at(-25 * HOUR))}
          firstName="Dana"
        />
      </StoryProviders>,
    );

    expect(screen.getByText("overdue 1 days")).toBeDefined();
  });

  it('keeps "You" on a task the reader holds themselves', async () => {
    // The writer assigns every human-written task to its author, so this is
    // the ordinary case, not a corner: a rule reading "assigned" as "somebody
    // else's" would strip the prefix from nearly every task on the page.
    const view = viewWithTask(null);
    // biome-ignore lint/style/noNonNullAssertion: the fixture above builds it.
    view.next_steps!.data[0].assignee_id = VIEWER;
    render(
      <StoryProviders>
        <PersonCommitmentsCard view={view} firstName="Dana" />
      </StoryProviders>,
    );

    // The reader's identity arrives with /me, so the prefix appears on the
    // render after it resolves.
    await waitFor(() =>
      expect(
        screen.getAllByText(/You: Send the MCP whitepaper/).length,
      ).toBeGreaterThan(0),
    );
  });

  it("does not tell the reader they owe a colleague's task", () => {
    const view = viewWithTask(null);
    // biome-ignore lint/style/noNonNullAssertion: the fixture above builds it.
    view.next_steps!.data[0].assignee_id = "u-someone-else";
    render(
      <StoryProviders>
        <PersonCommitmentsCard view={view} firstName="Dana" />
      </StoryProviders>,
    );

    expect(screen.queryByText(/You: Send the MCP whitepaper/)).toBeNull();
    expect(
      screen.getAllByText(/Send the MCP whitepaper/).length,
    ).toBeGreaterThan(0);
  });

  it("names a task whose subject it may not see", () => {
    const view = viewWithTask(null);
    // biome-ignore lint/style/noNonNullAssertion: the fixture above builds it.
    view.next_steps!.data[0].subject = null;
    render(
      <StoryProviders>
        <PersonCommitmentsCard view={view} firstName="Dana" />
      </StoryProviders>,
    );

    expect(screen.getAllByText(/an open task/).length).toBeGreaterThan(0);
  });

  it("still says nothing is promised when there is neither claim nor task", () => {
    render(
      <StoryProviders>
        <PersonCommitmentsCard
          view={{
            as_of: NOW,
            person: { id: "p-1", full_name: "Dana Buyer", ...CAPTURED },
            sections_omitted: [],
          }}
          firstName="Dana"
        />
      </StoryProviders>,
    );

    expect(screen.getByText(/Nothing has been promised/)).toBeDefined();
  });
});
