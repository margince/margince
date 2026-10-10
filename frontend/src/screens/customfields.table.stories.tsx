// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { Panel } from "../design-system/panel";
import {
  AuditRail,
  type CustomField,
  FieldTable,
  STAGED_ID,
} from "./customfields.table";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The field table stands straight in its Panel, as the card mounts it, so the
// row hairlines and the hover run to the pane's edge.
const meta: Meta = {
  title: "Settings/Sales/Fields/Custom field table",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

const field = (over: Partial<CustomField> = {}): CustomField => ({
  id: "01J2Z3K4M5N6P7Q8R9S0T1U2V3",
  object: "deal",
  label: "Renewal date",
  slug: "renewal_date",
  type: "date",
  status: "active",
  column_name: "cf_renewal_date",
  created_by: "u1",
  created_at: "2026-06-22T14:09:00Z",
  updated_at: "2026-06-22T14:09:00Z",
  version: 1,
  ...over,
});

const FIELDS: CustomField[] = [
  field(),
  field({
    id: "01J2Z3K4M5N6P7Q8R9S0T1U2V4",
    label: "Reason the stage moved back to qualification",
    slug: "stage_reason",
    type: "picklist",
    column_name: "cf_stage_reason",
    options: ["Budget", "Timing", "Champion left"],
    created_by: "u-2",
    version: 2,
  }),
  field({
    id: "01J2Z3K4M5N6P7Q8R9S0T1U2V5",
    label: "Ceiling",
    slug: "ceiling",
    type: "currency",
    column_name: "cf_ceiling",
    currency: "EUR",
    version: 1,
  }),
  field({
    id: "01J2Z3K4M5N6P7Q8R9S0T1U2V6",
    label: "Legacy priority",
    slug: "legacy_priority",
    type: "number",
    column_name: "cf_legacy_priority",
    status: "retired",
    created_by: "u-2",
  }),
];

const noop = () => {};

function Table({
  fields = FIELDS,
  canEdit = true,
}: Readonly<{ fields?: CustomField[]; canEdit?: boolean }>) {
  installFetchStub({
    "GET /users/names": () =>
      jsonResponse({ data: [{ id: "u-2", display_name: "Anna Weber" }] }),
  });
  return (
    <StoryProviders>
      <Panel title="Custom fields">
        <FieldTable
          object="deal"
          fields={fields}
          canEdit={canEdit}
          meUserId="u1"
          onRename={noop}
          onArchive={noop}
        />
      </Panel>
    </StoryProviders>
  );
}

/** Live fields, one retired: the retired row is badged and offers no verb. */
export const WithFields: Story = {
  render: () => <Table />,
};

export const WithFieldsDark: Story = {
  globals: { theme: "dark" },
  render: () => <Table />,
};

/** A live row's two verbs, in the one menu at its end. */
export const RowMenu: Story = {
  render: () => <Table />,
  play: async ({ canvasElement }) => {
    const body = within(canvasElement.ownerDocument.body);
    await userEvent.click(
      await body.findByRole("button", { name: "Actions for Ceiling" }),
    );
    await expect(
      await body.findByRole("button", { name: "Archive field" }),
    ).toBeVisible();
  },
};

/** A field being added: its row says so until the server commits it. */
export const Staged: Story = {
  render: () => (
    <Table
      fields={[
        ...FIELDS.slice(0, 1),
        field({ id: STAGED_ID, label: "Contract end", slug: "contract_end" }),
      ]}
    />
  ),
};

/** A seat that may read the catalogue and change nothing: no menus. */
export const NoPermission: Story = {
  render: () => <Table canEdit={false} />,
};

/** At a phone width each row folds: the field and its menu, then type and author. */
export const WithFieldsPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: () => <Table />,
};

type AuditLogEntry = components["schemas"]["AuditLogEntry"];

const change = (over: Partial<AuditLogEntry>): AuditLogEntry => ({
  id: "a-1",
  actor_type: "human",
  actor_id: "u1",
  actor_name: "Test User",
  action: "create",
  entity_type: "custom_field",
  entity_id: "01J2Z3K4M5N6P7Q8R9S0T1U2V3",
  occurred_at: "2026-06-22T14:09:00Z",
  ...over,
});

/** The change trail, one line per change with a hairline between them. */
export const ChangeTrail: Story = {
  render: () => (
    <StoryProviders>
      <AuditRail
        entries={[
          change({}),
          change({
            id: "a-2",
            action: "update",
            occurred_at: "2026-07-02T09:30:00Z",
          }),
          change({
            id: "a-3",
            action: "archive",
            actor_id: "u-2",
            actor_name: "Anna Weber",
            occurred_at: "2026-08-11T16:45:00Z",
          }),
        ]}
        state="ready"
        meUserId="u1"
        onRetry={noop}
      />
    </StoryProviders>
  ),
};
