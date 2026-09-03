// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../../api/schema";
import { installFetchStub, meRoute, StoryProviders } from "../story-utils";
import { SeatPerson } from "./seatperson";

// One stakeholder seat's person, in both readings a deal record can meet.
//
// The two stories are read as a pair, because the defect was drawing them
// alike: three cards on the deal page print this cell, and all three printed
// the name as flat text — so the only surface naming the people on a deal
// opened none of them. What the catalog is for here is seeing that a seat this
// reader may read is a link, and that the withheld one is a sentence with no
// control in it at all.

type Seat = components["schemas"]["DealCoverageSeat"];

const meta: Meta<typeof SeatPerson> = {
  title: "Records/Deal seat person",
  component: SeatPerson,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => {
      // A seat carries its own name, so EntityRef's lookup stays switched off
      // and nothing here should reach the network. The session route is what
      // keeps that honest: an unrouted probe reads as a malformed session, and
      // the harness says so out loud rather than letting the cell draw whatever
      // a failed-closed grant leaves behind.
      installFetchStub({ "GET /me": meRoute({}) });
      return (
        <StoryProviders>
          <Story />
        </StoryProviders>
      );
    },
  ],
};
export default meta;

type Story = StoryObj<typeof SeatPerson>;

function seat(over: Partial<Seat>): Seat {
  return {
    person_id: "01a03000-0000-7000-8000-0000000000b1",
    role: "champion",
    engaged: true,
    ...over,
  };
}

/** A seat this reader may read: the name is also the door to the person. */
export const NamedPerson: Story = {
  args: { seat: seat({ person_name: "Dana Weiss" }) },
};

// The case the component exists to get right, and the one to look at.
//
// `person_id` is always on the wire and only the NAME is withheld, so an href
// could be built here — and it would offer a route to a record the API answers
// 404 for precisely so that its existence stays hidden. A link would be an
// existence oracle drawn as a courtesy, so this cell has to render as prose.
export const WithheldPerson: Story = {
  args: {
    seat: seat({
      person_id: "01a03000-0000-7000-8000-0000000000b2",
      role: "economic_buyer",
    }),
  },
};
