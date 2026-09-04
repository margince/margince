// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { HeldThreadsCard } from "./held-threads";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// What a mailbox is holding back from the team. The states worth a picture are
// the two a reader must tell apart at a glance: a thread a classifier JUDGED,
// and one it has not answered about at all — which during an outage is every
// row on the page.

type HeldThread = components["schemas"]["HeldThread"];

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
  },
  {
    thread_key: "t-2",
    status: "held",
    pending: false,
    attempts: 1,
    has_message: true,
    kind: "legal",
    subject: "Entwurf Aufhebungsvertrag",
    activity_id: "01a05500-0000-7000-8000-0000000000a2",
    occurred_at: "2026-08-29T15:40:00Z",
  },
  {
    thread_key: "t-3",
    status: "unsure",
    pending: false,
    attempts: 2,
    has_message: true,
    subject: "Re: Termin",
    activity_id: "01a05500-0000-7000-8000-0000000000a3",
    occurred_at: "2026-08-28T11:02:00Z",
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
    });
    return (
      <StoryProviders>
        <HeldThreadsCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof HeldThreadsCard> = {
  title: "Settings/You/Held threads",
  component: HeldThreadsCard,
};
export default meta;

type Story = StoryObj<typeof HeldThreadsCard>;

export const Mixed: Story = { render: story(MIXED) };

// Nothing held is a READING, not an absent list: "my mailbox is withholding
// nothing right now" is what an owner opens this card to confirm.
export const NothingHeld: Story = { render: story([]) };

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
};
