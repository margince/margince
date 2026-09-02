// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { CaptureSendersCard } from "./capture-senders";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// What the classifier decided about a mailbox's senders. The states worth a
// picture are the ones a reader has to tell apart at a glance: a machine
// verdict, a verdict the seat OVERRULED, and a mailbox that has decided
// nothing yet.

type SenderDecision = components["schemas"]["CaptureSenderDecision"];

// One of each kind the classifier can answer with, so both halves of the
// decision column are visible in one frame: the three admitted kinds carry the
// tone, the rest read as the absence they are.
const EVERY_KIND: SenderDecision[] = [
  {
    address: "jana@commercetools.com",
    kind: "person",
    status: "real",
    overruled: false,
    record_exists: true,
  },
  {
    address: "info@steireif.de",
    kind: "role_mailbox",
    status: "unsure",
    overruled: false,
    record_exists: true,
  },
  {
    address: "anne@hotmail.com",
    kind: "personal",
    status: "noise",
    overruled: false,
    record_exists: false,
  },
  {
    address: "office@studiolegal.de",
    kind: "advisor",
    status: "real",
    overruled: false,
    record_exists: true,
  },
  {
    address: "receipts@expensify.com",
    kind: "transactional",
    status: "noise",
    overruled: false,
    record_exists: false,
  },
  {
    address: "news@substack.com",
    kind: "newsletter",
    status: "noise",
    overruled: false,
    record_exists: false,
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
  },
  {
    address: "anne@hotmail.com",
    kind: "personal",
    decision: "keep_out",
    overruled_kind: "personal",
    overruled: true,
    record_exists: false,
  },
  ...EVERY_KIND.slice(0, 2),
];

function story(rows: SenderDecision[]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /capture/senders": () => jsonResponse({ data: rows }),
    });
    return (
      <StoryProviders>
        <CaptureSendersCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof CaptureSendersCard> = {
  title: "Settings/You/Connections/Senders",
  component: CaptureSendersCard,
};
export default meta;
type Story = StoryObj<typeof CaptureSendersCard>;

export const EveryKind: Story = { render: story(EVERY_KIND) };

export const Overruled: Story = { render: story(OVERRULED) };

// A mailbox that has brought in nothing, or brought in only mail whose senders
// were all already known. The card still stands, because its absence would read
// as the feature being off.
export const NothingDecided: Story = { render: story([]) };
