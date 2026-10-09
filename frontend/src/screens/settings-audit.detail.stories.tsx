// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { AuditDetail } from "./settings-audit.detail";
import type { AuditLogEntry } from "./settings-audit.format";
import { StoryProviders } from "./story-utils";

const UPDATE: AuditLogEntry = {
  id: "al-1",
  actor_type: "agent",
  actor_id: "agent:sdr",
  passport_id: "01a11ea4-9c2e-7d10-a1b2-3c4d5e6f7a8b",
  on_behalf_of: "u-lars",
  on_behalf_of_name: "Lars Brandt",
  action: "update",
  entity_type: "deal",
  entity_id: "01a11ea4-0eed-704d-b84d-89ffa4435b4a",
  entity_label: "Demo GmbH renewal 2027",
  before: { stage: "proposal", amount_minor: 4_200_000, next_step: null },
  after: {
    stage: "negotiation",
    amount_minor: 4_800_000,
    next_step: "Send the revised order form",
  },
  authorization_rule: "role[rep] deal.update row_scope=own",
  evidence: { snippet: "Reply confirmed budget", source: "email:msg-1" },
  occurred_at: "2026-10-09T08:00:00Z",
};

const CREATED: AuditLogEntry = {
  ...UPDATE,
  id: "al-2",
  actor_type: "human",
  passport_id: null,
  evidence: null,
  action: "create",
  entity_type: "onboarding_wizard_state",
  entity_label: null,
  before: null,
  after: {
    step: "complete",
    voice_skipped: true,
    settings: { voice: false, connect: "later" },
  },
  authorization_rule:
    "role[individual] onboarding_wizard_state.create row_scope=own",
};

const REMOVED: AuditLogEntry = {
  ...CREATED,
  id: "al-3",
  action: "delete",
  entity_type: "contact",
  before: { full_name: "Priya Shah", email: "priya@demo.example" },
  after: null,
  authorization_rule: "role[admin,manager] contact.delete row_scope=all",
};

const LONG: AuditLogEntry = {
  ...CREATED,
  id: "al-4",
  after: {
    description:
      "A long description that keeps going well past the width of the column so the value has to wrap onto several lines without pushing the table sideways.",
    custom_fields: {
      procurement_contact: "einkauf@brandt-industrieanlagen.example",
      framework_agreement_reference: "FA-2026-000123-DACH-SONDERMASCHINEN",
    },
  },
  authorization_rule: "system",
};

const meta: Meta<typeof AuditDetail> = {
  title: "Settings/Governance/Audit log/Change detail",
  component: AuditDetail,
  render: (args) => (
    <StoryProviders>
      <AuditDetail {...args} />
    </StoryProviders>
  ),
};
export default meta;
type Story = StoryObj<typeof AuditDetail>;

export const Update: Story = { args: { entry: UPDATE } };

export const Created: Story = { args: { entry: CREATED } };

export const Removed: Story = { args: { entry: REMOVED } };

export const LongValues: Story = { args: { entry: LONG } };

export const UnknownRule: Story = {
  args: { entry: { ...UPDATE, authorization_rule: "role:admin" } },
};

export const UpdateDark: Story = {
  globals: { theme: "dark" },
  args: { entry: UPDATE },
};
