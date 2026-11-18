// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { DealStatusCardPanel } from "./dealstatus";

type DealStatusCard = components["schemas"]["DealStatusCard"];

const request = "00000000-0000-7000-8000-000000000001";
const reminder = "00000000-0000-7000-8000-000000000002";
const asOf = "2026-08-24T12:00:00Z";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The card the server answers once our reply is the newest message on the
// request's thread. It has no reply_to, and our_reply names the reminder the
// move opens.
function repliedCard(): DealStatusCard {
  return {
    deal_id: "demo-deal",
    story: {
      sentences: [{ text: "The customer asked for prices.", evidence: [] }],
    },
    our_reply: {
      request_activity_id: request,
      replied_at: "2026-08-21T18:00:00Z",
      replied_precision: "instant",
      reminder_id: reminder,
      reminder_version: 4,
    },
    next: {
      action: "open_task",
      reason:
        "You replied 3 days ago. If nothing more is owed, mark the reminder done.",
      arguments: { activity_id: reminder },
      evidence: [
        {
          activity_id: request,
          text: "Request: Prices",
          occurred_precision: "instant",
        },
      ],
    },
    generated_at: asOf,
    generated_by: "deterministic",
  };
}

function show(
  card: DealStatusCard,
  grants: string[],
  onPatch = vi.fn((_body: unknown) =>
    jsonResponse({ id: reminder, version: 5 }),
  ),
) {
  installFetchStub({
    "GET /me": meRoute({ activity: grants }),
    "GET /deals/demo-deal/status": () => jsonResponse(card),
    "GET /deals/demo-deal/coverage": () =>
      jsonResponse({ stakeholders: [], risks: [] }),
    [`PATCH /activities/${reminder}`]: (body) => onPatch(body),
  });
  render(
    <StoryProviders>
      <DealStatusCardPanel
        deal={{ id: "demo-deal", name: "Demo deal" }}
        onOpenEmail={vi.fn()}
      />
    </StoryProviders>,
  );
  return onPatch;
}

it("offers Done on the reminder once we replied, and marks it done", async () => {
  const patch = show(repliedCard(), ["read", "create", "update"]);
  await userEvent.click(await screen.findByRole("button", { name: "Done" }));
  await waitFor(() => expect(patch).toHaveBeenCalledWith({ is_done: true }));
  expect(
    screen.getByRole("button", { name: "Open the message" }),
  ).toBeVisible();
});

it("opens the reminder instead of offering Done to a reader who may not write it", async () => {
  show(repliedCard(), ["read"]);
  expect(await screen.findByText(/You replied 3 days ago/)).toBeVisible();
  expect(
    screen.queryByRole("button", { name: "Done" }),
  ).not.toBeInTheDocument();
});

it("keeps the move's own verb while the buyer's message is the newest", async () => {
  const card = repliedCard();
  card.our_reply = undefined;
  card.reply_to = request;
  show(card, ["read", "create", "update"]);
  expect(
    await screen.findByRole("button", { name: "Open task" }),
  ).toBeVisible();
  expect(
    screen.queryByRole("button", { name: "Done" }),
  ).not.toBeInTheDocument();
});
