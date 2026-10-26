// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { CalendarDays, History, Mail } from "lucide-react";
import { Button } from "./atoms";
import { Meter } from "./readings";
import { ServiceList, ServiceRow } from "./servicerow";

// The services one account brings in, in each state a service can be in.
// The same rows also appear as the permissions a connect flow asks for.

const meta: Meta<typeof ServiceRow> = {
  title: "Components/Layout and structure/Service row",
  component: ServiceRow,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ServiceRow>;

const services = (
  <ServiceList label="What this account brings in">
    <ServiceRow
      icon={Mail}
      name="New mail"
      caption="Mail with your customers, as it arrives. Synced 4 minutes ago."
      status={{ tone: "success", label: "Connected" }}
    />
    <ServiceRow
      icon={History}
      name="Past email"
      caption="Mail from before you connected, back to a date you pick."
      status={{ tone: "default", label: "Importing", live: true }}
    >
      <Meter value={1240} max={4800} label="Past email imported" dense />
    </ServiceRow>
    <ServiceRow
      icon={CalendarDays}
      name="Calendar"
      caption="Your meetings, and the times guests can book."
      status={{ tone: "default", label: "Not connected" }}
      action={<Button>Connect calendar</Button>}
    />
  </ServiceList>
);

export const Services: Story = { render: () => services };

// What a connect flow asks to read: no status and no verb, only the words.
export const Permissions: Story = {
  render: () => (
    <ServiceList label="What Margince asks to read">
      <ServiceRow
        icon={Mail}
        name="Read your mail"
        caption="To log conversations with your customers."
      />
      <ServiceRow
        icon={CalendarDays}
        name="Read and write your calendar"
        caption="To log meetings and offer booking times."
      />
    </ServiceList>
  ),
};

// One amber badge and one reason line; the verb sits on the account's row.
export const NeedsAttention: Story = {
  render: () => (
    <ServiceList label="What this account brings in">
      <ServiceRow
        icon={Mail}
        name="New mail"
        caption="Mail with your customers, as it arrives. Synced 2 days ago."
        status={{ tone: "warning", label: "Needs attention" }}
        reason="Google stopped sharing this mailbox. Reconnect the account to go on."
      />
      <ServiceRow
        icon={CalendarDays}
        name="Calendar"
        caption="Your meetings, and the times guests can book."
        status={{ tone: "danger", label: "Failed" }}
        reason="The last sync failed. Margince tries again in a few minutes."
      />
    </ServiceList>
  ),
};

export const Dark: Story = {
  render: () => services,
  globals: { theme: "dark" },
};

export const Phone: Story = {
  render: () => services,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
