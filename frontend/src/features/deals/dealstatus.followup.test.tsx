// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { sessionOptions } from "../../lib/session";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
  storyQueryClient,
} from "../../testing/story-utils";
import { DealStatusCardPanel } from "./dealstatus";

const created = "00000000-0000-7000-8000-0000000000aa";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("names the follow-up it created and opens that task, not the deal", async () => {
  const task = {
    id: created,
    kind: "task",
    subject: "Agree the next step",
    is_done: false,
    writable: true,
    version: 1,
    occurred_at: "2026-08-24T12:00:00Z",
    created_at: "2026-08-24T12:00:00Z",
    updated_at: "2026-08-24T12:00:00Z",
  };
  installFetchStub({
    "GET /me": meRoute({ activity: ["read", "create", "update"] }),
    "GET /deals/demo-deal/status": () =>
      jsonResponse({
        deal_id: "demo-deal",
        story: { sentences: [] },
        next: {
          action: "create_task",
          reason: "Agree the next step",
          arguments: { subject: "Agree the next step", source: "manual" },
          evidence: [],
        },
        generated_at: "2026-08-24T12:00:00Z",
        generated_by: "deterministic",
      }),
    "GET /deals/demo-deal/coverage": () =>
      jsonResponse({ stakeholders: [], risks: [] }),
    "POST /tasks": () => jsonResponse(task, 201),
    [`GET /activities/${created}`]: () => jsonResponse(task),
  });
  const user = userEvent.setup();
  render(
    <StoryProviders>
      <DealStatusCardPanel
        deal={{ id: "demo-deal", name: "Demo deal" }}
        onOpenEmail={vi.fn()}
      />
    </StoryProviders>,
  );
  await user.click(
    await screen.findByRole("button", { name: "Create follow-up" }),
  );
  expect(await screen.findByText("Follow-up created")).toBeVisible();
  await user.click(screen.getByRole("button", { name: "Open task" }));
  expect(await screen.findByRole("dialog")).toHaveTextContent(
    "Agree the next step",
  );
});

// The server's MoveRefusal is the one answer to whether a seat may take the
// move. The card draws its refusal, with its reason, as a control that is off.
it("draws the follow-up the server refused as off, with its reason, and writes no task", async () => {
  const reason = "You do not have permission to add tasks";
  const writes: string[] = [];
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /deals/demo-deal/status": () =>
      jsonResponse({
        deal_id: "demo-deal",
        story: { sentences: [] },
        next: {
          action: "create_task",
          blocked_reason: reason,
          reason: "Agree the next step",
          arguments: { subject: "Agree the next step", source: "manual" },
          evidence: [],
        },
        generated_at: "2026-08-24T12:00:00Z",
        generated_by: "deterministic",
      }),
    "GET /deals/demo-deal/coverage": () =>
      jsonResponse({ stakeholders: [], risks: [] }),
    "POST /tasks": () => {
      writes.push("POST /tasks");
      return jsonResponse({}, 201);
    },
  });
  render(
    <StoryProviders>
      <DealStatusCardPanel
        deal={{ id: "demo-deal", name: "Demo deal" }}
        onOpenEmail={vi.fn()}
      />
    </StoryProviders>,
  );

  expect(await screen.findByText("Agree the next step")).toBeVisible();
  const button = await screen.findByRole("button", {
    name: "Create follow-up",
  });
  expect(button).toBeDisabled();
  expect(button).toHaveAccessibleDescription(reason);
  await userEvent.click(button);
  expect(writes).toEqual([]);
});

it("draws the follow-up draft the server refused as off, with its reason", async () => {
  const reason = "You do not have permission to send email";
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /deals/demo-deal/status": () =>
      jsonResponse({
        deal_id: "demo-deal",
        story: { sentences: [] },
        next: {
          action: "draft_email",
          blocked_reason: reason,
          reason: "Follow up on the meeting",
          arguments: {
            activity_id: "m-1",
            links: [{ entity_type: "contact", entity_id: "p-1" }],
          },
          evidence: [],
        },
        generated_at: "2026-08-24T12:00:00Z",
        generated_by: "deterministic",
      }),
    "GET /deals/demo-deal/coverage": () =>
      jsonResponse({ stakeholders: [], risks: [] }),
  });
  render(
    <StoryProviders>
      <DealStatusCardPanel
        deal={{ id: "demo-deal", name: "Demo deal" }}
        onOpenEmail={vi.fn()}
      />
    </StoryProviders>,
  );

  expect(await screen.findByText("Follow up on the meeting")).toBeVisible();
  const button = await screen.findByRole("button", {
    name: /draft follow-up/i,
  });
  expect(button).toBeDisabled();
  expect(button).toHaveAccessibleDescription(reason);
});

it("offers the follow-up the server did not refuse, whatever the session says", async () => {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /deals/demo-deal/status": () =>
      jsonResponse({
        deal_id: "demo-deal",
        story: { sentences: [] },
        next: {
          action: "create_task",
          reason: "Agree the next step",
          arguments: { subject: "Agree the next step", source: "manual" },
          evidence: [],
        },
        generated_at: "2026-08-24T12:00:00Z",
        generated_by: "deterministic",
      }),
    "GET /deals/demo-deal/coverage": () =>
      jsonResponse({ stakeholders: [], risks: [] }),
  });
  const client = storyQueryClient();
  render(
    <StoryProviders client={client}>
      <DealStatusCardPanel
        deal={{ id: "demo-deal", name: "Demo deal" }}
        onOpenEmail={vi.fn()}
      />
    </StoryProviders>,
  );
  await screen.findByText("Agree the next step");
  // Asserted once the session is in, when a client-side grant check would
  // already have hidden the verb.
  await waitFor(() =>
    expect(client.getQueryData(sessionOptions.queryKey)).toBeDefined(),
  );
  expect(
    screen.getByRole("button", { name: "Create follow-up" }),
  ).toBeEnabled();
});
