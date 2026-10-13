// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Badge, Button, Card } from "./atoms";
import { Checklist, ChecklistStep } from "./checklist";
import { DataTable } from "./datatable";

// Numbered steps toward a working setup: one with every state, and one where
// the product cannot see progress, so every step stays open with a body.

const meta: Meta<typeof Checklist> = {
  title: "Components/Status indicators/Checklist",
  component: Checklist,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof Checklist>;

export const MixedStates: Story = {
  render: () => (
    <Card title="Your setup">
      <Checklist>
        <ChecklistStep
          state="done"
          title="Connect your mail"
          caption="admin@demo.test, Gmail"
          action={<Badge tone="success">Done</Badge>}
        />
        <ChecklistStep
          state="current"
          title="Connect your calendar"
          caption="Meetings on deals and your booking link need it."
          action={<Button variant="primary">Connect calendar</Button>}
        />
        <ChecklistStep
          state="open"
          title="Train your writing voice"
          caption="Drafts sound like you once it has a few of your mails."
          action={<Button>Start</Button>}
        />
        <ChecklistStep
          state="open"
          title="Share your booking link"
          caption="Waits on an administrator to turn on bookings."
        />
      </Checklist>
    </Card>
  ),
};
export const MixedStatesDark: Story = {
  ...MixedStates,
  globals: { theme: "dark" },
};

type RedirectRow = Readonly<{ id: string; address: string }>;
const REDIRECTS: RedirectRow[] = [
  { id: "signin", address: "https://crm.example.com/auth/google/callback" },
  { id: "mail", address: "https://crm.example.com/connect/google/callback" },
];

export const AllOpenWithBodies: Story = {
  render: () => (
    <Card title="Set up the Google app">
      <Checklist>
        <ChecklistStep
          state="open"
          title="Create an OAuth client in Google Cloud"
          caption="Choose Web application as the type."
        />
        <ChecklistStep state="open" title="Add these redirect addresses">
          <DataTable<RedirectRow>
            label="Redirect addresses"
            rows={REDIRECTS}
            rowKey={(row) => row.id}
            columns={[
              {
                key: "address",
                header: "Address",
                render: (row) => row.address,
              },
            ]}
          />
        </ChecklistStep>
        <ChecklistStep
          state="open"
          title="Paste the client ID and secret"
          caption="Margince keeps the secret encrypted and never shows it again."
        />
      </Checklist>
    </Card>
  ),
};
export const AllOpenWithBodiesDark: Story = {
  ...AllOpenWithBodies,
  globals: { theme: "dark" },
};
