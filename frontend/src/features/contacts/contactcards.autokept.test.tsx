/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { UTC_ZONE } from "../../format/timezone";
import { pretendReaderZone } from "../../testing/readerzone.testkit";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactCommitmentsCard } from "./contactcards";

// The screen reads the browser's zone here, so the suite names it.
beforeEach(() => pretendReaderZone(UTC_ZONE));

type Contact360 = components["schemas"]["Contact360"];
type ConversationClaim = components["schemas"]["ConversationClaim"];

// A promise one of us made that nobody was asked to keep: open, no task, no
// taker. The card offers it to a reader who may hold tasks.

const OWNERLESS: ConversationClaim = {
  id: "c-1",
  kind: "commitment_ours",
  body: "Send the signed contract",
  source_activity_id: "a-1",
  occurred_precision: "instant",
  source_quote: "We will send the signed contract by Friday.",
  status: "open",
  needs_review: false,
  due_at: null,
};

const CAPTURED = {
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
} as const;

function view(claims: ConversationClaim[], tasks: string[] = []): Contact360 {
  return {
    as_of: "2026-08-25T12:00:00Z",
    contact: { id: "p-1", full_name: "Dana Buyer", ...CAPTURED },
    sections_omitted: [],
    claims,
    next_steps: {
      data: tasks.map((id) => ({
        offers_email: false,
        id,
        kind: "task",
        subject: "Send the signed contract",
        occurred_at: "2026-08-25T12:00:00Z",
        occurred_precision: "instant",
        is_done: false,
        ...CAPTURED,
      })),
      page: { has_more: false },
    },
  };
}

function renderCard(claims: ConversationClaim[], tasks: string[] = []) {
  render(
    <StoryProviders>
      <ContactCommitmentsCard
        view={view(claims, tasks)}
        firstName="Dana"
        onAllTasks={() => {}}
      />
    </StoryProviders>,
  );
}

afterEach(cleanup);

const KEPT: ConversationClaim = {
  ...OWNERLESS,
  status: "done",
  settled_automatically: true,
};

describe("a promise Margince marked kept", () => {
  it("says so and reopens it with one press", async () => {
    const pressed: string[] = [];
    installFetchStub({
      "GET /me": meRoute({ activity: ["read"], contact: ["read", "update"] }),
      "POST /claims/c-1/reopen": () => {
        pressed.push("c-1");
        return new Response(null, { status: 204 });
      },
    });
    renderCard([KEPT]);
    expect(await screen.findByText(/Margince marked this kept/)).toBeTruthy();
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Undo" }));
    await waitFor(() => expect(pressed).toEqual(["c-1"]));
  });

  it("offers no undo to a reader who may not update contacts", async () => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read"], contact: ["read"] }),
    });
    renderCard([KEPT]);
    expect(await screen.findByText(/Margince marked this kept/)).toBeTruthy();
    await act(async () => {});
    expect(screen.queryByRole("button", { name: "Undo" })).toBeNull();
  });

  it("is not claimed for a promise a person settled", async () => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read"], contact: ["read", "update"] }),
    });
    renderCard([{ ...KEPT, settled_automatically: false }]);
    await screen.findByRole("heading", { name: "Commitments" });
    await act(async () => {});
    expect(screen.queryByText(/Margince marked this kept/)).toBeNull();
  });
});
