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

const LOOKS_KEPT: ConversationClaim = {
  ...OWNERLESS,
  looks_kept: {
    meeting_id: "m-1",
    guest_name: "Riley Reed",
    meeting_at: "2026-08-30T10:00:00Z",
  },
};

describe("a promise that looks kept", () => {
  it("names who accepted which meeting and marks it kept through the ordinary settle", async () => {
    const settled: unknown[] = [];
    installFetchStub({
      "GET /me": meRoute({ activity: ["read"], contact: ["read", "update"] }),
      "POST /claims/c-1/settle": (body) => {
        settled.push(body);
        return new Response(null, { status: 204 });
      },
    });
    renderCard([LOOKS_KEPT]);
    expect(
      await screen.findByText(
        /Looks kept: Riley Reed accepted the 30 Aug meeting/,
      ),
    ).toBeTruthy();
    await userEvent
      .setup()
      .click(await screen.findByRole("button", { name: "Mark kept" }));
    await waitFor(() => expect(settled).toEqual([{ outcome: "done" }]));
  });

  it("offers no button to a reader who may not update contacts", async () => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read"], contact: ["read"] }),
    });
    renderCard([LOOKS_KEPT]);
    expect(await screen.findByText(/Looks kept/)).toBeTruthy();
    await act(async () => {});
    expect(screen.queryByRole("button", { name: "Mark kept" })).toBeNull();
  });

  it("says nothing for a promise with no hint", async () => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read"], contact: ["read", "update"] }),
    });
    renderCard([OWNERLESS]);
    await screen.findByRole("heading", { name: "Commitments" });
    await act(async () => {});
    expect(screen.queryByText(/Looks kept/)).toBeNull();
  });
});
