/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import { taskRow } from "../../testing/brief.fixtures";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { Triage } from "./brief.focus";

afterEach(cleanup);

type WorklistItem = components["schemas"]["WorklistItem"];

const LEAD: WorklistItem = {
  ...taskRow("task-1", "Call back Alice about the renewal"),
  contact: { id: "c-1", label: "Alice Brandt" },
};

function showLead(lead: WorklistItem) {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }, { roles: ["rep"] }),
    "GET /activities/task-1": () =>
      jsonResponse({
        id: "task-1",
        kind: "task",
        subject: LEAD.title,
        occurred_at: "2026-09-08T09:00:00Z",
        is_done: false,
        source_activity_id: "call-1",
        links: [{ entity_type: "contact", entity_id: "c-1" }],
      }),
    "GET /activities/call-1": () =>
      jsonResponse({
        id: "call-1",
        kind: "call",
        content_state: "available",
        occurred_at: "2026-09-08T08:00:00Z",
      }),
    "GET /contacts/c-1": () =>
      jsonResponse({
        id: "c-1",
        full_name: "Alice Brandt",
        primary_email: "alice@example.com",
        phones: [{ id: "ph-1", phone: "+491517654321", is_primary: true }],
      }),
  });
  render(
    <StoryProviders>
      <Triage
        rows={[lead]}
        at={0}
        lead={lead}
        onChoose={() => {}}
        onOpenEmail={() => {}}
        onContext={() => {}}
      />
    </StoryProviders>,
  );
}

it("a task lead from a call offers Call and Email, and links the contact once", async () => {
  showLead(LEAD);

  const card = within(await screen.findByRole("article"));
  const dial = await card.findByRole("link", { name: en["record.verb.call"] });
  expect(dial.getAttribute("href")).toBe("tel:+491517654321");
  expect(
    card.getByRole("link", { name: en["record.verb.email"] }),
  ).toBeTruthy();
  const toContact = card
    .getAllByRole("link")
    .filter((link) => link.getAttribute("href") === "#/contacts/c-1");
  expect(toContact).toHaveLength(1);
});

it("a task lead whose row names no contact still offers the task's contact verbs", async () => {
  showLead({ ...LEAD, contact: undefined });

  const card = within(await screen.findByRole("article"));
  expect(
    await card.findByRole("link", { name: en["record.verb.call"] }),
  ).toBeTruthy();
  expect(
    card.getByRole("link", { name: en["record.verb.email"] }),
  ).toBeTruthy();
});
