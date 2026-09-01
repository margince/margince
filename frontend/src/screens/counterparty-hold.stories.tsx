// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { CounterpartyHoldRow } from "./counterparty-hold";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// The hold control has two states worth a picture, and they are not the same
// control with a flag: one OFFERS two verbs, the other reports a standing
// decision and how to undo it. The second also carries the sentence a reader
// would otherwise learn the hard way — that lifting does not re-open what was
// already held.

const HELD_DOMAIN = {
  id: "hold-1",
  kind: "domain",
  value: "studiolegal.de",
  created_at: "2026-08-01T09:00:00Z",
};

function story(holds: unknown[]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /capture/counterparty-holds": () => jsonResponse({ data: holds }),
    });
    return (
      <StoryProviders>
        <CounterpartyHoldRow email="office@studiolegal.de" />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof CounterpartyHoldRow> = {
  title: "Records/Rail/Counterparty hold",
  component: CounterpartyHoldRow,
};
export default meta;
type Story = StoryObj<typeof CounterpartyHoldRow>;

export const NotHeld: Story = { render: story([]) };

// A DOMAIN hold covering this address, which is the one worth having for an
// advisor: the firm answers from whichever address picked up the file, so the
// row reports the domain it is actually held by rather than the address asked
// about.
export const HeldByDomain: Story = { render: story([HELD_DOMAIN]) };

export const HeldByAddress: Story = {
  render: story([
    { ...HELD_DOMAIN, kind: "address", value: "office@studiolegal.de" },
  ]),
};
