// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../../api/schema";
import { Panel } from "../../design-system/panel";
import { installFetchStub, meRoute, StoryProviders } from "../story-utils";
import { ThreadFold } from "./glance";

// The thread folded inside the account's 360.
//
// It arrives CLOSED, so the row itself is the thing to judge: it has to say
// what is inside — how many exchanges, and the newest one — well enough that a
// reader can decide whether opening it is worth the half-page it costs. The
// second frame is that page, which is the only way to see that the teaser and
// the first row of the list say the same thing rather than two things.
//
// It folds inside the 360's lead card, so it is drawn here inside a panel: on
// its own it would float with no band above it and the summary's measure would
// be nobody's.

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

function Fold({ data }: Readonly<{ data: Company360 }>) {
  installFetchStub({ "GET /me": meRoute({ company: ["read"] }) });
  return (
    <StoryProviders>
      <div style={{ maxWidth: 640 }}>
        <Panel title="Nordwind Logistik">
          <ThreadFold
            view={data}
            loading={false}
            onOpenHistory={() => {}}
            onOpenRecord={() => {}}
          />
        </Panel>
      </div>
    </StoryProviders>
  );
}

const meta: Meta = {
  title: "Records/Company record/Thread fold",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

// Closed, which is how a reader meets it: the count, the newest exchange and
// its day, with the way to the History tab outside the toggle.
export const Teased: Story = {
  render: () => <Fold data={view([inboundEmail, loggedCall])} />,
};

// The toggle is the `<summary>` of a `<details>`, which carries no button role
// of its own — the whole row is the control, so the name it wears is what a
// reader presses and what these plays reach for.
const openTheFold =
  (name: string): NonNullable<Story["play"]> =>
  async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(await canvas.findByText(name));
  };

// Opened. The teaser's exchange is the list's first row, and the reading it
// teased is now the message itself.
export const Opened: Story = {
  render: () => <Fold data={view([inboundEmail, loggedCall])} />,
  play: openTheFold("What happened · 2"),
};

// An account nothing has been filed against: the fold opens on the section's
// own empty state rather than teasing an exchange it cannot promise.
export const NothingLogged: Story = {
  render: () => <Fold data={view([])} />,
  play: openTheFold("What happened"),
};
