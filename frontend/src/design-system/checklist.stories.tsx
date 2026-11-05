// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Badge, Button, Card } from "./atoms";
import { Checklist, ChecklistStep } from "./checklist";
import { DataTable } from "./datatable";

// Numbered steps toward a working setup: every state of the list, the count
// caption and the inline trail of a flow. One frame is a list where the
// product cannot see progress, so every step stays open with a body.

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
          state="waiting"
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

/** A step nobody here can move: the grey badge takes the action's place, in
 *  the default words or the caller's. */
export const WithWaiting: Story = {
  render: () => (
    <Card title="Bookings">
      <Checklist>
        <ChecklistStep
          state="current"
          title="Set your working hours"
          caption="Bookings only offer times inside them."
          action={<Button variant="primary">Set hours</Button>}
        />
        <ChecklistStep
          state="waiting"
          title="Turn on bookings for the workspace"
          caption="An administrator turns this on in Settings."
        />
        <ChecklistStep
          state="waiting"
          waitingLabel="Invited"
          title="Your teammate joins the round robin"
          caption="Mai Tran has the invitation."
        />
      </Checklist>
    </Card>
  ),
};

export const WithCount: Story = {
  render: () => (
    <Card title="Your setup">
      <Checklist count={{ done: 2, total: 4 }}>
        <ChecklistStep
          state="done"
          title="Connect your mail"
          action={<Badge tone="success">Done</Badge>}
        />
        <ChecklistStep
          state="done"
          title="Connect your calendar"
          action={<Badge tone="success">Done</Badge>}
        />
        <ChecklistStep
          state="current"
          title="Train your writing voice"
          action={<Button variant="primary">Start</Button>}
        />
        <ChecklistStep state="open" title="Share your booking link" />
      </Checklist>
    </Card>
  ),
};

function ConnectTrail() {
  return (
    <Checklist variant="inline" label="Steps">
      <ChecklistStep state="done" title="Account" />
      <ChecklistStep state="current" title="What to bring in" />
      <ChecklistStep state="open" title="Sign in" />
    </Checklist>
  );
}

/** The connect flow's trail: where the reader is in steps walked with Back
 *  and Continue, so no step carries a body or a verb. */
export const Inline: Story = { render: () => <ConnectTrail /> };
export const InlineDark: Story = {
  ...Inline,
  globals: { theme: "dark" },
};

/** The trail at a phone's width, where it wraps instead of scrolling. The
 *  `uat-phone` tag is what drives the capture gate's browser to 390px. */
export const InlinePhone: Story = {
  render: () => (
    <div style={{ padding: "var(--padCard)" }}>
      <ConnectTrail />
    </div>
  ),
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

/** A long title at a phone's width: the action wraps under the text instead
 *  of squeezing the title into a narrow column beside it. */
export const ActionWrapsPhone: Story = {
  ...MixedStates,
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
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
