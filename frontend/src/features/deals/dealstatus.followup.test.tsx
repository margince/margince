// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
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

it("offers no follow-up to a seat that may not write activities", async () => {
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
  render(
    <StoryProviders>
      <DealStatusCardPanel
        deal={{ id: "demo-deal", name: "Demo deal" }}
        onOpenEmail={vi.fn()}
      />
    </StoryProviders>,
  );

  // The move is on screen; only its verb is withheld.
  expect(await screen.findByText("Agree the next step")).toBeVisible();
  await waitFor(() =>
    expect(
      screen.queryByRole("button", { name: "Create follow-up" }),
    ).not.toBeInTheDocument(),
  );
});
