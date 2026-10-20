// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../../api/schema";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../../testing/story-utils";
import { LatestSection } from "./glance";

// What happened on the account lately, as the overview's Latest section.

type Company360 = components["schemas"]["Company360"];
type Activity = components["schemas"]["Activity"];

const EMAIL_ID = "01a05500-0000-7000-8000-00000000dd01";

const inboundEmail: Activity = {
  id: EMAIL_ID,
  kind: "email",
  occurred_at: "2026-08-29T09:15:00Z",
  subject: "Re: the renewal quote",
  body: "Can you hold the price until Friday?",
  direction: "inbound",
  content_state: "available",
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-08-29T09:15:00Z",
  updated_at: "2026-08-29T09:15:00Z",
  version: 4,
  is_done: false,
};

const loggedCall: Activity = {
  id: "01a05500-0000-7000-8000-00000000dd02",
  kind: "call",
  occurred_at: "2026-08-26T13:40:00Z",
  subject: "Walked through the retrofit scope",
  direction: "outbound",
  content_state: "available",
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-08-26T13:40:00Z",
  updated_at: "2026-08-26T13:40:00Z",
  version: 1,
  is_done: false,
};

function view(activities: Activity[]): Company360 {
  return {
    as_of: "2026-08-29T10:00:00Z",
    company: {
      id: "o-1",
      display_name: "Nordwind Logistik",
      source: "manual",
      captured_by: "human:u-1",
      created_at: "2026-01-05T09:00:00Z",
      updated_at: "2026-08-29T09:15:00Z",
    },
    sections_omitted: [],
    activities: { data: activities, page: { has_more: false } },
  };
}

function Latest({ data }: Readonly<{ data: Company360 }>) {
  installFetchStub({ "GET /me": meRoute({ company: ["read"] }) });
  return (
    <StoryProviders>
      <div style={{ maxWidth: 640 }}>
        <LatestSection
          view={data}
          loading={false}
          onOpenHistory={() => {}}
          onOpenRecord={() => {}}
          onOpenEmail={() => {}}
        />
      </div>
    </StoryProviders>
  );
}

const meta: Meta = {
  title: "Records/Company 360/Latest",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

// The newest exchanges first, with the way to the History tab beside the title.
export const Exchanges: Story = {
  render: () => <Latest data={view([inboundEmail, loggedCall])} />,
};

// An account nothing has been filed against: the section's own empty state.
export const NothingLogged: Story = {
  render: () => <Latest data={view([])} />,
};
