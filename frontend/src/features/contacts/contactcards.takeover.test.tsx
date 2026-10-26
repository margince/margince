/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ContactCommitmentsCard } from "./contactcards";

type Contact360 = components["schemas"]["Contact360"];
type ConversationClaim = components["schemas"]["ConversationClaim"];

// A promise one of us made that nobody was asked to keep: open, no task, no
// taker. The card offers it to a reader who may hold tasks.

const OWNERLESS: ConversationClaim = {
  id: "c-1",
  kind: "commitment_ours",
  body: "Send the signed contract",
  source_activity_id: "a-1",
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

describe("a promise nobody was asked to keep", () => {
  it("is taken over with one press", async () => {
    const pressed: string[] = [];
    installFetchStub({
      "GET /me": meRoute({ activity: ["read", "create"], contact: ["read"] }),
      "POST /claims/c-1/take-over": () => {
        pressed.push("c-1");
        return jsonResponse({
          claim_id: "c-1",
          task_activity_id: "t-1",
          taken_by_user_id: "u-2",
          taken_by_name: "Priya Raman",
          taken_at: "2026-08-25T12:00:00Z",
        });
      },
    });
    renderCard([OWNERLESS]);
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "I’ll take this" }),
    );
    await waitFor(() => expect(pressed).toEqual(["c-1"]));
  });

  it("names the colleague who pressed first when the press is refused", async () => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read", "create"], contact: ["read"] }),
      "POST /claims/c-1/take-over": () =>
        jsonResponse(
          {
            status: 409,
            code: "conflict",
            detail: "Priya Raman took this promise over on 2026-08-25",
          },
          409,
        ),
    });
    renderCard([OWNERLESS]);
    const user = userEvent.setup();
    await user.click(
      await screen.findByRole("button", { name: "I’ll take this" }),
    );
    expect(await screen.findByText(/Priya Raman/)).toBeTruthy();
  });

  it("is not offered to a reader who may not hold tasks", async () => {
    let meServed = false;
    installFetchStub({
      "GET /me": () => {
        meServed = true;
        return meRoute({ activity: ["read"], contact: ["read"] })();
      },
    });
    renderCard([OWNERLESS]);
    await waitFor(() => expect(meServed).toBe(true));
    await act(async () => {});
    expect(screen.queryByRole("button", { name: "I’ll take this" })).toBeNull();
  });

  it("is not offered once it is somebody's task", async () => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read", "create"], contact: ["read"] }),
    });
    renderCard([{ ...OWNERLESS, task_activity_id: "t-1" }], ["t-1"]);
    // The open task is the promise now, and the Tasks tab lists it.
    await screen.findByRole("heading", { name: "Commitments" });
    await act(async () => {});
    expect(screen.queryByRole("button", { name: "I’ll take this" })).toBeNull();
  });
});

describe("a promise a colleague took over", () => {
  it("says who took it and when, once, in place of its task", async () => {
    installFetchStub({
      "GET /me": meRoute({ activity: ["read", "create"], contact: ["read"] }),
    });
    renderCard(
      [
        {
          ...OWNERLESS,
          task_activity_id: "t-1",
          taken_by_user_id: "u-2",
          taken_by_name: "Priya Raman",
          taken_at: "2026-08-25T12:00:00Z",
        },
      ],
      ["t-1"],
    );
    expect(
      await screen.findByText("Priya Raman took this on 25 Aug"),
    ).toBeTruthy();
    expect(screen.getAllByRole("checkbox")).toHaveLength(1);
    expect(screen.queryByRole("button", { name: "I’ll take this" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Dismiss" })).toBeNull();
  });
});
