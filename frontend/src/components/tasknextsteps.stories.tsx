// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { within } from "storybook/test";
import type { components } from "../api/schema";
import { en } from "../i18n/en";
import { WriteToProvider } from "../lib/writeto";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { TaskNextSteps } from "./tasknextsteps";

// Check: Call only on a task from a call, and a refused Email says why
// beside it. A task with no single contact drafts to its lead or company.

const meta: Meta = {
  title: "Patterns/Task next steps",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const TASK: components["schemas"]["Activity"] = {
  occurred_precision: "instant",
  offers_email: false,
  id: "task-1",
  kind: "task",
  subject: "Call back Alice about the renewal",
  occurred_at: "2026-09-08T09:00:00Z",
  is_done: false,
  source_activity_id: "call-1",
  links: [{ entity_type: "contact", entity_id: "c-1" }],
  email_target: { entity_type: "contact", entity_id: "c-1" },
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-09-08T09:00:00Z",
  updated_at: "2026-09-08T09:00:00Z",
};

const TARGET = { entity_type: "contact", entity_id: "c-1" } as const;

const ALICE = {
  id: "c-1",
  full_name: "Alice Brandt",
  primary_email: "alice@brandt.example",
  phones: [{ id: "ph-1", phone: "+491517654321", is_primary: true }],
  archived_at: null,
};

const ON_LEAD: components["schemas"]["Activity"] = {
  ...TASK,
  subject: "Email Lena about the pilot",
  source_activity_id: null,
  links: [{ entity_type: "lead", entity_id: "l-1" }],
  email_target: { entity_type: "lead", entity_id: "l-1" },
};

function Steps({
  task = TASK,
  contact = {},
  composer = false,
}: Readonly<{
  task?: components["schemas"]["Activity"];
  contact?: Record<string, unknown>;
  composer?: boolean;
}>) {
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /activities/call-1": () =>
      jsonResponse({
        id: "call-1",
        kind: "call",
        content_state: "available",
        occurred_at: "2026-09-08T08:00:00Z",
      }),
    "GET /contacts/c-1": () => jsonResponse({ ...ALICE, ...contact }),
  });
  return (
    <StoryProviders>
      <WriteToProvider writeTo={composer ? () => {} : null}>
        <TaskNextSteps task={task} className="form-actions" />
      </WriteToProvider>
    </StoryProviders>
  );
}

const offers = (name: string) => async () => {
  await within(document.body).findByRole("link", { name });
};

export const FromCall: Story = {
  render: () => <Steps />,
  play: offers(en["record.verb.call"]),
};

export const TypedByHand: Story = {
  render: () => <Steps task={{ ...TASK, source_activity_id: null }} />,
  play: offers(en["task.next.openContact"]),
};

export const NoAddress: Story = {
  render: () => (
    <Steps
      contact={{ primary_email: null }}
      task={{ ...TASK, email_target: { ...TARGET, refusal: "no_address" } }}
    />
  ),
  play: offers(en["record.verb.call"]),
};

export const NoPhone: Story = {
  render: () => <Steps contact={{ phones: [] }} />,
  play: offers(en["record.verb.email"]),
};

export const ArchivedContact: Story = {
  render: () => (
    <Steps
      contact={{ archived_at: "2026-09-01T00:00:00Z" }}
      task={{ ...TASK, email_target: { ...TARGET, refusal: "archived" } }}
    />
  ),
  play: offers(en["task.next.openContact"]),
};

export const OnALead: Story = {
  render: () => <Steps task={ON_LEAD} composer />,
  play: async () => {
    await within(document.body).findByRole("button", {
      name: en["task.next.draftEmail"],
    });
  },
};

export const OnAClosedLead: Story = {
  render: () => (
    <Steps
      task={{
        ...ON_LEAD,
        email_target: {
          entity_type: "lead",
          entity_id: "l-1",
          refusal: "lead_closed",
        },
      }}
    />
  ),
  play: async () => {
    await within(document.body).findByText(en["lead.terminalReadOnly"]);
  },
};
