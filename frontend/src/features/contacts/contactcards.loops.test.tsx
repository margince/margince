/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactClaimReadings, ContactCommitmentsCard } from "./contactcards";
import { hasOpenCommitments } from "./contactoverview.rows";

type Contact360 = components["schemas"]["Contact360"];
type ConversationClaim = components["schemas"]["ConversationClaim"];

// The Commitments card reads a promise against the clock. It used to need a
// full elapsed day before it said overdue. For 24 hours it called a promise
// "due yesterday" that the task list called late. These pin the boundary at
// the instant.

const CAPTURED: Pick<
  Contact360["contact"],
  "source" | "captured_by" | "created_at" | "updated_at"
> = {
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
};

beforeEach(() => {
  installFetchStub({ "GET /me": meRoute({ activity: ["read"] }) });
});

// Fixed, because a suite that reads the machine's calendar changes its
// verdict in December.
const NOW = "2026-08-25T12:00:00Z";
const HOUR = 3_600_000;

function claim(over: Partial<ConversationClaim> = {}): ConversationClaim {
  return {
    id: "c-1",
    kind: "commitment_ours",
    body: "Send the pilot quote",
    source_activity_id: "a-1",
    occurred_precision: "instant",
    source_quote: "I'll send the quote on Monday.",
    status: "open",
    needs_review: false,
    due_at: null,
    ...over,
  };
}

function viewWith(claims: ConversationClaim[]): Contact360 {
  return {
    as_of: NOW,
    contact: { id: "p-1", full_name: "Dana Buyer", ...CAPTURED },
    sections_omitted: [],
    claims,
  };
}

function at(offsetMs: number): string {
  return new Date(Date.parse(NOW) + offsetMs).toISOString();
}

function renderCard(view: Contact360, onAllTasks = () => undefined) {
  render(
    <StoryProviders>
      <ContactCommitmentsCard
        view={view}
        firstName="Dana"
        onAllTasks={onAllTasks}
      />
    </StoryProviders>,
  );
}

function renderDue(dueAt: string | null) {
  renderCard(viewWith([claim({ due_at: dueAt })]));
}

afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe("the Commitments card's due pill", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(NOW));
  });

  it("calls a promise overdue the moment it passes, not a day later", () => {
    renderDue(at(-23 * HOUR));
    expect(screen.getByText("Overdue by less than a day")).toBeTruthy();
    expect(screen.queryByText(/^Due /)).toBeNull();
  });

  it("counts whole days only once a whole day has passed", () => {
    renderDue(at(-49 * HOUR));
    expect(screen.getByText("Overdue 2 days")).toBeTruthy();
  });

  it("says one day in the singular", () => {
    renderDue(at(-25 * HOUR));
    expect(screen.getByText("Overdue 1 day")).toBeTruthy();
  });

  it("is not overdue at the instant it falls due", () => {
    renderDue(at(0));
    expect(screen.getByText("Due today")).toBeTruthy();
  });

  it("keeps a promise still hours away under today, not tomorrow", () => {
    renderDue(at(11 * HOUR));
    expect(screen.getByText("Due today")).toBeTruthy();
  });

  it("names tomorrow only for the day after this one", () => {
    renderDue(at(26 * HOUR));
    expect(screen.getByText("Due tomorrow")).toBeTruthy();
  });

  it("reads an unreadable due instant as no date at all", () => {
    renderDue("whenever");
    expect(screen.getByText("Open")).toBeTruthy();
  });

  it("marks what they owe as waiting on them", () => {
    renderCard(viewWith([claim({ kind: "commitment_theirs" })]));
    expect(screen.getByText("Waiting")).toBeTruthy();
  });
});

describe("the Commitments card's rows", () => {
  it("prefixes our promise with Team and theirs with their first name", () => {
    renderCard(
      viewWith([
        claim(),
        claim({
          id: "c-2",
          kind: "commitment_theirs",
          body: "confirm the attendees",
        }),
      ]),
    );
    expect(
      screen.getAllByText("Team: Send the pilot quote").length,
    ).toBeGreaterThan(0);
    expect(
      screen.getAllByText("Dana: confirm the attendees").length,
    ).toBeGreaterThan(0);
  });

  it("shows the words a promise was read from under it", () => {
    renderCard(viewWith([claim()]));
    expect(screen.getByText("“I'll send the quote on Monday.”")).toBeTruthy();
  });

  it("has no button inside a row: dismissal lives on the Research tab", () => {
    renderCard(viewWith([claim()]));
    expect(screen.queryByRole("button", { name: "Dismiss" })).toBeNull();
  });

  it("ticks a kept promise and names no deadline for it", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date(NOW));
    renderCard(
      viewWith([
        claim({ status: "done", task_activity_id: "t-1", due_at: at(-HOUR) }),
      ]),
    );
    expect(
      screen.getByRole("checkbox", { name: /Send the pilot quote/ }),
    ).toHaveProperty("checked", true);
    expect(screen.queryByText(/Overdue/)).toBeNull();
  });

  it("leaves a promise that became an open task to the task", () => {
    const view = viewWith([claim({ task_activity_id: "t-1" })]);
    view.next_steps = {
      data: [
        {
          occurred_precision: "instant",
          offers_email: false,
          id: "t-1",
          kind: "task",
          subject: "Send the pilot quote",
          occurred_at: NOW,
          is_done: false,
          ...CAPTURED,
        },
      ],
      page: { has_more: false },
    };
    renderCard(view);
    expect(screen.queryByText(/Send the pilot quote/)).toBeNull();
  });

  it("says nothing is promised when nothing is", () => {
    renderCard(viewWith([]));
    expect(screen.getByText(/No commitments or questions/)).toBeTruthy();
  });

  it("says the claims are hidden rather than absent when withheld", () => {
    renderCard({
      ...viewWith([]),
      claims: undefined,
      sections_omitted: ["claims"],
    });
    expect(screen.getByText("Hidden for your role")).toBeTruthy();
    expect(screen.queryByText(/No commitments or questions/)).toBeNull();
  });

  it("opens the task queue from All tasks", async () => {
    const onAllTasks = vi.fn();
    renderCard(viewWith([claim()]), onAllTasks);
    (await screen.findByRole("button", { name: "All tasks" })).click();
    expect(onAllTasks).toHaveBeenCalledOnce();
  });
});

// Dismissing a reading is a judgement about the extractor, made beside the
// words it was read from.
describe("the claim readings on the Research tab", () => {
  it("shows each open reading with its words and dismisses it", async () => {
    const settled: unknown[] = [];
    installFetchStub({
      "GET /me": meRoute({ activity: ["read"], contact: ["read", "update"] }),
      "POST /claims/c-1/settle": (body) => {
        settled.push(body);
        return new Response(null, { status: 204 });
      },
    });
    render(
      <StoryProviders>
        <ContactClaimReadings view={viewWith([claim()])} />
      </StoryProviders>,
    );
    expect(screen.getByText("“I'll send the quote on Monday.”")).toBeTruthy();
    (await screen.findByRole("button", { name: "Dismiss" })).click();
    await waitFor(() => expect(settled).toEqual([{ outcome: "dismissed" }]));
  });

  it("offers no dismissal to a reader who may not update the contact", async () => {
    let meServed = false;
    installFetchStub({
      "GET /me": () => {
        meServed = true;
        return meRoute({ activity: ["read"], contact: ["read"] })();
      },
    });
    render(
      <StoryProviders>
        <ContactClaimReadings view={viewWith([claim()])} />
      </StoryProviders>,
    );
    await screen.findByText("“I'll send the quote on Monday.”");
    await waitFor(() => expect(meServed).toBe(true));
    await act(async () => {});
    expect(screen.queryByRole("button", { name: "Dismiss" })).toBeNull();
  });

  it("draws nothing once every reading is settled", () => {
    const { container } = render(
      <StoryProviders>
        <ContactClaimReadings view={viewWith([claim({ status: "done" })])} />
      </StoryProviders>,
    );
    expect(container.querySelector(".panel")).toBeNull();
  });
});

describe("responsibility and historical conversation findings", () => {
  it("does not assign a team claim to the reader", () => {
    renderDue(null);
    expect(
      screen.getByRole("checkbox", { name: "Team: Send the pilot quote" }),
    ).toBeVisible();
    expect(screen.queryByText(/You: Send the pilot quote/)).toBeNull();
  });

  it("keeps an unverified old finding out of open obligations while retaining its source", async () => {
    const user = userEvent.setup();
    const old = claim({ due_at: "2024-07-08T09:00:00Z" });
    old.needs_review = true;
    old.occurred_at = "2024-07-05T09:00:00Z";
    old.source_label = "Rollout planning";
    const view = viewWith([old]);
    expect(hasOpenCommitments(view)).toBe(false);
    render(
      <StoryProviders>
        <ContactCommitmentsCard
          view={view}
          firstName="Dana"
          onAllTasks={() => undefined}
        />
      </StoryProviders>,
    );
    expect(
      screen.queryByRole("checkbox", { name: /Send the pilot quote/ }),
    ).toBeNull();
    expect(screen.queryByText(/overdue/)).toBeNull();
    await user.click(screen.getByText("Review conversation findings"));
    expect(screen.getByText("Send the pilot quote")).toBeVisible();
    expect(screen.getByText(/Rollout planning/)).toBeVisible();
    expect(screen.getByText(/I'll send the quote on Monday/)).toBeVisible();
    expect(screen.queryByText(/You: Send the pilot quote/)).toBeNull();
    expect(screen.queryByText(/overdue/)).toBeNull();
  });
});
