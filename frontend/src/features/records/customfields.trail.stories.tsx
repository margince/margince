// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { FieldChangeTrail } from "./customfields.trail";

// The change trail as the card mounts it, closed until the reader opens it.
// Open, it has one row per change: its record type, field and what it did.
const meta: Meta = {
  title: "Settings/Sales/Records and fields/Recent field changes",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

type AuditLogEntry = components["schemas"]["AuditLogEntry"];
type CustomField = components["schemas"]["CustomField"];

const budget = (object: CustomField["object"]): CustomField => ({
  id: `budget-${object}`,
  object,
  label: "Budget",
  slug: "budget",
  type: "number",
  status: object === "deal" ? "retired" : "active",
  column_name: "cf_budget",
  show_in_create: true,
  reportable: false,
  created_by: "u1",
  created_at: "2026-06-22T14:09:00Z",
  updated_at: "2026-06-22T14:09:00Z",
});

const change = (
  object: CustomField["object"],
  over: Partial<AuditLogEntry>,
): AuditLogEntry => ({
  id: `a-${object}-${over.occurred_at}`,
  actor_type: "human",
  actor_id: "u1",
  actor_name: "Test User",
  action: "update",
  entity_type: "custom_field",
  entity_id: `budget-${object}`,
  occurred_at: "2026-06-22T14:09:00Z",
  ...over,
});

// The same label on three record types, then three edits that keep it.
const ENTRIES: AuditLogEntry[] = [
  change("contact", {
    action: "create",
    after: { object: "contact", label: "Budget" },
  }),
  change("company", {
    action: "create",
    occurred_at: "2026-06-22T14:10:00Z",
    after: { object: "company", label: "Budget" },
  }),
  change("deal", {
    action: "create",
    occurred_at: "2026-06-22T14:11:00Z",
    after: { object: "deal", label: "Budget" },
  }),
  change("contact", {
    occurred_at: "2026-07-02T09:30:00Z",
    before: { label_translations: {} },
    after: { label_translations: { de: "Etat" } },
  }),
  change("company", {
    actor_id: "u-2",
    actor_name: "Anna Weber",
    occurred_at: "2026-08-11T16:45:00Z",
    before: { show_in_create: true },
    after: { show_in_create: false },
  }),
  change("deal", {
    occurred_at: "2026-08-12T08:05:00Z",
    before: { status: "active" },
    after: { status: "retired" },
  }),
];

const OBJECTS = ["contact", "company", "deal"] as const;

const routes = (allow: Parameters<typeof meRoute>[0]) => ({
  "GET /me": meRoute(allow),
  "GET /audit-log": () => jsonResponse({ data: ENTRIES }),
  "GET /custom-fields": (_body: unknown, url: URL) => {
    const object = OBJECTS.find(
      (one) => one === url.searchParams.get("object"),
    );
    return jsonResponse({ data: object ? [budget(object)] : [] });
  },
});

const open: Story["play"] = async ({ canvasElement }) => {
  await userEvent.click(
    await within(canvasElement).findByText(en["cf.audit.title"]),
  );
};

export const Opened: Story = {
  render: () => {
    installFetchStub(routes({ audit_log: ["read"] }));
    return (
      <StoryProviders>
        <FieldChangeTrail meUserId="u1" />
      </StoryProviders>
    );
  },
  play: open,
};

export const OpenedDark: Story = {
  ...Opened,
  globals: { theme: "dark" },
};

// A role that may not read the audit log still sees the section, withheld.
export const Withheld: Story = {
  render: () => {
    installFetchStub(routes({}));
    return (
      <StoryProviders>
        <FieldChangeTrail meUserId="u1" />
      </StoryProviders>
    );
  },
  play: open,
};
