// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Calendar, Mail, Server } from "lucide-react";
import { useState } from "react";
import { Button, Card, OverflowMenu } from "./atoms";
import { ProviderMark } from "./provider-mark";
import { ProviderList, ProviderRow } from "./providerrow";
import { Chip, Meter } from "./readings";
import { SettingList, SettingRow } from "./settingrow";
import { Switch } from "./switch";

// One row per connected account, in each state a connection can be in. The
// last stories put every state in one list, in each theme. Syncing is grey
// with a live dot: blue never says status.

const meta: Meta<typeof ProviderRow> = {
  title: "Components/Layout and structure/Provider row",
  component: ProviderRow,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ProviderRow>;

const services = (
  <>
    <Chip icon={Mail} dense>
      Mail
    </Chip>
    <Chip icon={Calendar} dense>
      Calendar
    </Chip>
  </>
);

const menu = (
  <OverflowMenu label="More actions for this connection">
    <Button>Change what it reads</Button>
    <Button variant="danger">Disconnect</Button>
  </OverflowMenu>
);

const notConnected = (
  <ProviderRow
    mark={<ProviderMark providerKey="microsoft" />}
    name="Microsoft 365"
    status={{ tone: "default", label: "Not connected" }}
    facts={services}
    action={<Button variant="primary">Connect</Button>}
  />
);

const syncing = (
  <ProviderRow
    mark={<ProviderMark providerKey="google" />}
    name="Google"
    status={{ tone: "default", label: "Syncing", live: true }}
    account="admin@demo.test"
    facts={
      <>
        {services}
        <span>Connected today at 10:38</span>
      </>
    }
    menu={menu}
  >
    <Meter value={1240} max={4800} label="Past email imported" dense />
  </ProviderRow>
);

const connected = (
  <ProviderRow
    mark={<ProviderMark providerKey="google" />}
    name="Google"
    status={{ tone: "success", label: "Connected" }}
    account="rep.one@brandt-automotive.example"
    facts={
      <>
        {services}
        <span>Synced 4 minutes ago</span>
      </>
    }
    menu={menu}
  />
);

const needsAttention = (
  <ProviderRow
    mark={<ProviderMark providerKey="microsoft" />}
    name="Microsoft 365"
    status={{ tone: "warning", label: "Needs attention" }}
    account="admin@demo.test"
    facts={services}
    reason="Microsoft stopped accepting the sign-in on 2 October."
    action={<Button variant="primary">Reconnect</Button>}
    menu={menu}
  />
);

const failed = (
  <ProviderRow
    mark={<ProviderMark providerKey="google" />}
    name="Google"
    status={{ tone: "danger", label: "Failed" }}
    account="admin@demo.test"
    facts={services}
    reason="The last three syncs could not reach Google."
    action={<Button>Try again</Button>}
    menu={menu}
  />
);

const imap = (
  <ProviderRow
    mark={<Server aria-hidden="true" />}
    name="Other mail server (IMAP)"
    status={{ tone: "success", label: "Connected" }}
    account="sales@brandt-automotive.example"
    facts={
      <Chip icon={Mail} dense>
        Mail
      </Chip>
    }
    menu={menu}
  />
);

export const NotConnected: Story = { render: () => notConnected };
export const Syncing: Story = { render: () => syncing };
export const Connected: Story = { render: () => connected };
export const NeedsAttention: Story = { render: () => needsAttention };
export const Failed: Story = { render: () => failed };
export const GlyphMark: Story = { render: () => imap };

// A name that opens a section of its own, with what hangs under it.
export const AsHeading: Story = {
  render: () => (
    <ProviderRow
      mark={<ProviderMark providerKey="google" />}
      name="Google"
      headingLevel="h3"
      status={{ tone: "success", label: "Connected" }}
      account="admin@demo.test"
      menu={menu}
    >
      <p className="t-caption">
        Mail and Calendar, both read every few minutes.
      </p>
    </ProviderRow>
  ),
};

// What a connected account carries under it: the services it reads, then the
// settings that belong to this account alone, both on the text column.
function NestedSections() {
  const [calendar, setCalendar] = useState(true);
  return (
    <ProviderRow
      mark={<ProviderMark providerKey="google" />}
      name="Google"
      headingLevel="h3"
      status={{ tone: "success", label: "Connected" }}
      account="admin@demo.test"
      menu={menu}
    >
      <ProviderList headingLevel="h4">
        <ProviderRow
          mark={<Mail aria-hidden="true" />}
          name="Mail"
          status={{ tone: "success", label: "Connected" }}
          facts="Synced 4 minutes ago"
        />
        <ProviderRow
          mark={<Calendar aria-hidden="true" />}
          name="Calendar"
          status={{ tone: "default", label: "Syncing", live: true }}
          facts="Reading the last 90 days"
        />
      </ProviderList>
      <SettingList>
        <SettingRow
          label="Log meetings from this calendar"
          description="Meetings with a customer land on their timeline."
          control={
            <Switch
              label="Log meetings from this calendar"
              labelHidden
              checked={calendar}
              onChange={setCalendar}
            />
          }
        />
      </SettingList>
    </ProviderRow>
  );
}
export const WithNestedSections: Story = { render: () => <NestedSections /> };

// A connect tile: the row alone in a Card, which pads it, so the row drops its
// own block padding.
export const InTile: Story = {
  render: () => (
    <ul style={{ listStyle: "none", margin: 0, padding: 0 }}>
      <Card as="li">
        <ProviderRow
          mark={<ProviderMark providerKey="google" />}
          name="Google"
          headingLevel="h3"
          facts="Mail and Calendar"
          action={<Button variant="primary">Connect</Button>}
        />
      </Card>
    </ul>
  ),
};

// Inside a ProviderList every name is an h3 under the card's h2, with no prop
// on the rows. A list one level deeper says `headingLevel="h4"` once, as
// WithNestedSections does.
export const InList: Story = {
  render: () => (
    <Card title="Mail and calendar">
      <ProviderList>
        {connected}
        {needsAttention}
        {notConnected}
      </ProviderList>
    </Card>
  ),
};

export const AllStates: Story = {
  render: () => (
    <Card title="Mail and calendar">
      <ProviderList>
        {connected}
        {syncing}
        {needsAttention}
        {failed}
        {imap}
        {notConnected}
      </ProviderList>
    </Card>
  ),
};
export const AllStatesDark: Story = {
  ...AllStates,
  globals: { theme: "dark" },
};
