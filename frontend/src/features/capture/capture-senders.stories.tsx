// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, waitFor, within } from "storybook/test";
import type { components } from "../../api/schema";
import { linkedInReachFixture } from "../../testing/reachfixture";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { ConnectionOutcomesCard } from "./connection-outcomes";

// The senders drawer, in the three states a reader tells apart at a glance: a
// machine verdict, a verdict the seat overruled, and nothing decided.

type SenderDecision = components["schemas"]["CaptureSenderDecision"];
type AwaitingContact = components["schemas"]["CaptureAwaitingContact"];

// One of each kind the classifier can answer with, so both halves of the
// decision column are visible in one frame: the three admitted kinds carry the
// tone, the rest read as the absence they are.
const EVERY_KIND: SenderDecision[] = [
  {
    address: "jana@commercetools.com",
    kind: "contact",
    status: "real",
    overruled: false,
    record_exists: true,
    contact_id: "0190b7a2-0000-7000-8000-000000000011",
    contact_name: "Jana Keller",
    message_count: 248,
    last_seen_at: "2026-10-09T16:20:00Z",
  },
  {
    address: "info@steireif.de",
    kind: "role_mailbox",
    status: "unsure",
    overruled: false,
    record_exists: true,
    contact_id: "0190b7a2-0000-7000-8000-000000000013",
    contact_name: "Steireif Office",
    message_count: 31,
    last_seen_at: "2026-09-28T07:05:00Z",
  },
  {
    address: "anne@hotmail.com",
    kind: "personal",
    status: "noise",
    overruled: false,
    record_exists: false,
    message_count: 4,
    last_seen_at: "2026-08-14T19:42:00Z",
  },
  {
    address: "office@studiolegal.de",
    kind: "advisor",
    status: "real",
    overruled: false,
    record_exists: true,
    contact_id: "0190b7a2-0000-7000-8000-000000000014",
    contact_name: "Studio Legal Partners",
    message_count: 12,
    last_seen_at: "2026-10-01T11:30:00Z",
  },
  {
    address: "receipts@expensify.com",
    kind: "transactional",
    status: "noise",
    overruled: false,
    record_exists: false,
    message_count: 1093,
    last_seen_at: "2026-10-11T23:58:00Z",
  },
  {
    address: "news@substack.com",
    kind: "newsletter",
    status: "noise",
    overruled: false,
    record_exists: false,
    message_count: 57,
    last_seen_at: "2026-10-12T06:00:00Z",
  },
  // A long address, and a kind this build has no word for.
  {
    address: "maximilian.oberstleutnant-von-hohenzollern@procurement.example",
    kind: "export_control",
    status: "unsure",
    overruled: false,
    record_exists: false,
    message_count: 0,
  },
  // A generated address with no break in it, cut to one line.
  {
    address:
      "bounces+srs=4f7a2c9e1b0d=mailer-relay-20261009-7731@notifications.example-mailer.com",
    kind: "transactional",
    status: "noise",
    overruled: false,
    record_exists: false,
    message_count: 212,
    last_seen_at: "2026-10-12T04:10:00Z",
  },
];

// The seat corrected the machine. The filled badge and the "you decided" clause
// are what separate this row from the quiet dots above it — a reader auditing
// the list needs to see which answers are theirs.
const OVERRULED: SenderDecision[] = [
  {
    address: "news@substack.com",
    kind: "newsletter",
    decision: "business",
    overruled_kind: "newsletter",
    overruled: true,
    record_exists: true,
    contact_id: "0190b7a2-0000-7000-8000-000000000015",
    contact_name: "Substack Editorial",
    message_count: 57,
    last_seen_at: "2026-10-12T06:00:00Z",
  },
  {
    address: "anne@hotmail.com",
    kind: "personal",
    decision: "keep_out",
    overruled_kind: "personal",
    overruled: true,
    record_exists: false,
    message_count: 4,
    last_seen_at: "2026-08-14T19:42:00Z",
  },
  ...EVERY_KIND.slice(0, 2),
];

function story(rows: SenderDecision[], waiting: AwaitingContact[] = []) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /capture/senders": () => jsonResponse({ data: rows }),
      "GET /capture/contacts-awaiting-decision": () =>
        jsonResponse({ data: waiting }),
      "GET /capture/held-threads": () => jsonResponse({ data: [] }),
      "GET /me/linkedin-reach": () => jsonResponse(linkedInReachFixture()),
    });
    return (
      <StoryProviders>
        <ConnectionOutcomesCard />
      </StoryProviders>
    );
  };
}

// The list opens from its row; the drawer is portalled to the body.
async function openList(canvasElement: HTMLElement, name: string) {
  await userEvent.click(
    await within(canvasElement).findByRole("button", { name: `View ${name}` }),
  );
  return within(
    await within(canvasElement.ownerDocument.body).findByRole("dialog", {
      name,
    }),
  );
}

const openSenders = async ({
  canvasElement,
}: Readonly<{ canvasElement: HTMLElement }>) => {
  await openList(canvasElement, "Senders");
};

const meta = {
  title: "Settings/You/Capture activity/Senders",
  component: ConnectionOutcomesCard,
} satisfies Meta<typeof ConnectionOutcomesCard>;
export default meta;
type Story = StoryObj<typeof meta>;

export const EveryKind: Story = {
  render: story(EVERY_KIND),
  play: openSenders,
};

export const Overruled: Story = { render: story(OVERRULED), play: openSenders };

// A mailbox that has brought in nothing: the row says so and offers no list.
export const NothingDecided: Story = {
  render: story([]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await canvas.findAllByText("None yet");
    await expect(
      canvas.queryByRole("button", { name: "View Senders" }),
    ).toBeNull();
  },
};

// Contacts still visible to this seat alone because nothing has decided their
// sender. The second one's address never reached the senders list above it,
// which is why the group is a read of its own: the admin card counts both.
const WAITING: AwaitingContact[] = [
  {
    contact_id: "0190b7a2-0000-7000-8000-000000000011",
    display_name: "Jana Keller",
    emails: ["jana@commercetools.com", "j.keller@commercetools.com"],
    captured_at: "2026-09-02T08:15:00Z",
  },
  {
    contact_id: "0190b7a2-0000-7000-8000-000000000012",
    emails: ["m.ortiz@steireif.de"],
    captured_at: "2026-09-10T14:40:00Z",
  },
];

export const WaitingOnDecision: Story = {
  render: story(EVERY_KIND.slice(0, 2), WAITING),
  play: async ({ canvasElement }) => {
    const drawer = await openList(canvasElement, "Waiting on a decision");
    await waitFor(() => expect(drawer.getByText("Jana Keller")).toBeVisible());
  },
};

// The row's verbs sit behind one menu at a fixed place.
export const RowMenu: Story = {
  render: story(OVERRULED),
  play: async ({ canvasElement }) => {
    const drawer = await openList(canvasElement, "Senders");
    await userEvent.click(
      drawer.getByRole("button", { name: "Actions for news@substack.com" }),
    );
    const page = within(canvasElement.ownerDocument.body);
    await waitFor(() =>
      expect(
        page.getByRole("button", { name: "Withdraw my answer" }),
      ).toBeVisible(),
    );
  },
};

// The drawer is a full sheet at 390px, and the table folds to two lines.
export const EveryKindPhone: Story = {
  render: story(EVERY_KIND, WAITING),
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: openSenders,
};
