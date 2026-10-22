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

// What a mailbox is holding back from the team. The states worth a picture are
// the two a reader must tell apart at a glance: a thread a classifier JUDGED,
// and one it has not answered about at all — which during an outage is every
// row on the page.

type HeldThread = components["schemas"]["HeldThread"];

const HELD_LEGAL: HeldThread = {
  thread_key: "t-2",
  status: "held",
  pending: false,
  attempts: 1,
  has_message: true,
  kind: "legal",
  subject: "Entwurf Aufhebungsvertrag",
  activity_id: "01a05500-0000-7000-8000-0000000000a2",
  occurred_at: "2026-08-29T15:40:00Z",
  // More outside addresses than the row names: the rest are counted.
  participants: [
    {
      address: "k.huber@kanzlei-huber-partner.example",
      contact_id: "0190b7a2-0000-7000-8000-0000000000c7",
    },
    { address: "office@kanzlei-huber-partner.example" },
    { address: "a.lindqvist@betriebsrat.example" },
    { address: "hr-legal@steireif.de" },
    { address: "assistenz@kanzlei-huber-partner.example" },
  ],
  participant_count: 8,
};

const MIXED: HeldThread[] = [
  // Pending leads, which is the ordering the endpoint guarantees and the whole
  // reason this page is worth opening during an outage.
  // Each readable row carries its activity_id beside its subject. The two come
  // through the same content clause on the server, so a fixture with one and
  // not the other draws the withheld reading over a subject it was given.
  {
    thread_key: "t-1",
    status: "pending",
    pending: true,
    attempts: 4,
    has_message: true,
    subject: "Angebot Q4 — Rückfragen",
    activity_id: "01a05500-0000-7000-8000-0000000000a1",
    occurred_at: "2026-08-30T09:12:00Z",
    participants: [
      {
        address: "m.ortiz@steireif.de",
        contact_id: "0190b7a2-0000-7000-8000-000000000012",
      },
    ],
    participant_count: 1,
  },
  HELD_LEGAL,
  {
    thread_key: "t-3",
    status: "unsure",
    pending: false,
    attempts: 2,
    has_message: true,
    subject: "Re: Termin",
    activity_id: "01a05500-0000-7000-8000-0000000000a3",
    occurred_at: "2026-08-28T11:02:00Z",
    // Only colleagues were on it, so the row names nobody.
    participants: [],
    participant_count: 0,
  },
  // A message held by this seat that this seat may not READ: somebody else's
  // personnel mail, on a thread they are holding. It exists — so it is not the
  // erased row below — and neither its subject nor its id crossed the content
  // clause, so it names itself and offers nothing.
  {
    thread_key: "t-5",
    status: "held",
    pending: false,
    attempts: 1,
    has_message: true,
    kind: "personnel",
    occurred_at: "2026-08-28T08:15:00Z",
  },
  // The verdict outlives its evidence: an erasure inside the window nulls the
  // activity and the hold stands. The row says the message is gone rather than
  // drawing a blank cell that reads as a message with no subject.
  {
    thread_key: "t-4",
    status: "held",
    pending: false,
    attempts: 1,
    // No message left: the release is refused with a reason rather than
    // offering a control whose only outcome is an error.
    has_message: false,
    kind: "personal",
  },
];

function story(rows: HeldThread[]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /capture/held-threads": () => jsonResponse({ data: rows }),
      "GET /capture/senders": () => jsonResponse({ data: [] }),
      "GET /capture/contacts-awaiting-decision": () =>
        jsonResponse({ data: [] }),
      "GET /me/linkedin-reach": () => jsonResponse(linkedInReachFixture()),
    });
    return (
      <StoryProviders>
        <ConnectionOutcomesCard />
      </StoryProviders>
    );
  };
}

// The list opens from its row, in a drawer portalled to the body.
async function openHeld(canvasElement: HTMLElement) {
  await userEvent.click(
    await within(canvasElement).findByRole("button", {
      name: "View Held threads",
    }),
  );
  return within(canvasElement.ownerDocument.body).findByRole("dialog", {
    name: "Held threads",
  });
}

const meta = {
  title: "Settings/You/Capture activity/Held threads",
  component: ConnectionOutcomesCard,
} satisfies Meta<typeof ConnectionOutcomesCard>;
export default meta;

type Story = StoryObj<typeof meta>;

// The drawer is narrower than the fold width, so who a thread is with sits
// on its own line under line one.
export const Mixed: Story = {
  render: story(MIXED),
  play: async ({ canvasElement }) => {
    const drawer = await openHeld(canvasElement);
    await within(drawer).findByRole("button", {
      name: "Entwurf Aufhebungsvertrag",
    });
    await waitFor(() => {
      const beside = drawer.querySelector(".held-threads-parties");
      expect(beside?.getBoundingClientRect().height).toBe(0);
      const cell = drawer.querySelector('td[data-fold="only"]:not(:empty)');
      expect(cell?.getBoundingClientRect().height).toBeGreaterThan(0);
    });
  },
};

// Nothing held is a READING, not an absent list: the row says so, with no
// list to open.
export const NothingHeld: Story = {
  render: story([]),
  play: async ({ canvasElement }) => {
    await within(canvasElement).findAllByText("None yet");
    await expect(
      within(canvasElement).queryByRole("button", {
        name: "View Held threads",
      }),
    ).toBeNull();
  },
};

// Every row pending, which is what a classifier outage looks like from the
// owner's side — and the state the attempts count exists for.
export const Outage: Story = {
  render: story(
    MIXED.map((row, i) => ({
      ...row,
      status: "pending",
      pending: true,
      kind: undefined,
      attempts: i + 1,
    })),
  ),
  play: async ({ canvasElement }) => {
    await openHeld(canvasElement);
  },
};

// A long subject truncates inside its column. A kind this build has no word
// for still reads as words, and a thread with no message left says so.
const EDGES: HeldThread[] = [
  {
    ...HELD_LEGAL,
    thread_key: "t-long",
    subject:
      "Quarterly procurement review: revised framework agreement, annex C pricing schedule and the open questions from legal about indemnity caps",
  },
  { ...HELD_LEGAL, thread_key: "t-new-kind", kind: "export_control" },
  {
    ...HELD_LEGAL,
    thread_key: "t-erased",
    has_message: false,
    subject: undefined,
    activity_id: undefined,
    occurred_at: undefined,
    participants: undefined,
    participant_count: undefined,
  },
];

export const Edges: Story = {
  render: story([...MIXED, ...EDGES]),
  play: async ({ canvasElement }) => {
    await openHeld(canvasElement);
  },
};

// Folded, the same addresses are a caption line of their own under line one.
export const MixedPhone: Story = {
  render: story([...MIXED, ...EDGES]),
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async ({ canvasElement }) => {
    const drawer = await openHeld(canvasElement);
    await within(drawer).findAllByRole("button", {
      name: "Entwurf Aufhebungsvertrag",
    });
    const beside = drawer.querySelector(".held-threads-parties");
    await expect(beside?.getBoundingClientRect().height).toBe(0);
    const cell = drawer.querySelector('td[data-fold="only"]:not(:empty)');
    const row = cell?.closest("tr")?.getBoundingClientRect();
    const box = cell?.getBoundingClientRect();
    await expect(box?.height).toBeGreaterThan(0);
    await expect(Math.round(box?.width ?? 0)).toBeGreaterThan(
      Math.round((row?.width ?? 0) / 2),
    );
  },
};
