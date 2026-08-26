// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { DealRoomAccess } from "./dealroomaccess";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

type DealRoom = components["schemas"]["DealRoom"];
type Participant = components["schemas"]["DealRoomParticipant"];

// Who has been admitted to a deal room, whether the invitation reached them,
// and what they have taken out of it since.
//
// Two facts on this card are easy to confuse and are modelled apart, so the
// stories keep them apart too. `delivery_state` is what happened to the LAST
// credential we sent — it moves every time one is resent. `has_signed_in` is
// whether this person ever exchanged one for a session, and it does not move,
// because that is the fact that fixes their address.
//
// The reading line is deliberately absent at zero. "0 documents" reads as a
// judgement about the buyer, and early in a room's life the honest report is
// that there is nothing to report — so `NobodyHasOpenedIt` is a story about a
// line that is not there.

const ROOM: DealRoom = {
  id: "room-1",
  deal_id: "deal-1",
  title: "Acme Expansion — Deal Room",
  state: "live",
  source: "manual",
  captured_by: "u-me",
  version: 1,
  created_at: "2026-08-22T09:00:00Z",
  updated_at: "2026-08-22T09:00:00Z",
};

function participant(overrides: Partial<Participant> = {}): Participant {
  return {
    id: "part-1",
    room_id: ROOM.id,
    full_name: "Dana Buyer",
    email: "dana.buyer@brandt-automotive.example",
    capability: "read",
    delivery_state: "delivered",
    has_signed_in: true,
    last_seen_at: "2026-08-24T14:02:00Z",
    source: "manual",
    captured_by: "u-me",
    created_at: "2026-08-22T09:10:00Z",
    updated_at: "2026-08-24T14:02:00Z",
    ...overrides,
  };
}

function access(rows: Participant[], mayManage = true) {
  return () => {
    installFetchStub({
      "GET /deal-rooms/room-1/participants": () =>
        jsonResponse({ data: rows, page: { next_cursor: null } }),
    });
    return (
      <StoryProviders>
        <DealRoomAccess room={ROOM} mayManage={mayManage} />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof DealRoomAccess> = {
  title: "Screens/Deal Room/Access",
  component: DealRoomAccess,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof DealRoomAccess>;

/** Nobody admitted yet. An empty room is a real state of a room a rep just
 *  opened, and it says so rather than drawing an empty list. */
export const NobodyAdmitted: Story = { render: access([]) };

/**
 * Admitted, and the room has been read. The reading line names how many
 * documents left the room and which — a count and its own evidence, so a
 * figure nobody can check is never the whole report.
 */
export const HasBeenRead: Story = {
  render: access([
    participant({
      download_count: 3,
      documents_downloaded: [
        "Commercial terms v4",
        "Implementation plan",
        "Security appendix",
      ],
    }),
  ]),
};

/**
 * A room that has been worked hard. Four digits is where de-DE first groups, so
 * this is the only story in which the count's notation is visible at all — and
 * the line sits beside document titles, which is exactly where a figure written
 * in the wrong notation reads as a different number.
 */
export const HeavilyRead: Story = {
  render: access([
    participant({
      download_count: 1204,
      documents_downloaded: ["Commercial terms v4", "Implementation plan"],
    }),
  ]),
};

/** The same room in German. */
export const HeavilyReadGerman: Story = {
  render: () => {
    installFetchStub({
      "GET /deal-rooms/room-1/participants": () =>
        jsonResponse({
          data: [
            participant({
              download_count: 1204,
              documents_downloaded: ["Kommerzielle Bedingungen v4"],
            }),
          ],
          page: { next_cursor: null },
        }),
    });
    return (
      <StoryProviders locale="de">
        <DealRoomAccess room={ROOM} mayManage />
      </StoryProviders>
    );
  },
};

/**
 * Admitted, and nothing taken. The reading line is ABSENT rather than reporting
 * zero — the state this card is most often in, and the one a "0 documents" line
 * would turn into an accusation.
 */
export const NobodyHasOpenedIt: Story = {
  render: access([participant({ has_signed_in: false, last_seen_at: null })]),
};

/**
 * The invitation bounced. Delivery is modelled apart from access for this
 * case: the person is admitted, and the credential never reached them, so the
 * row has to say which of the two went wrong.
 */
export const InvitationFailed: Story = {
  render: access([
    participant({
      delivery_state: "failed",
      has_signed_in: false,
      last_seen_at: null,
    }),
  ]),
};

/** Several people, in the states a real room mixes them in. */
export const SeveralPeople: Story = {
  render: access([
    participant({ download_count: 12, documents_downloaded: ["Terms v4"] }),
    participant({
      id: "part-2",
      full_name: "Jonas Petersen",
      email: "jonas.petersen@brandt-automotive.example",
      delivery_state: "sent",
      has_signed_in: false,
      last_seen_at: null,
    }),
    participant({
      id: "part-3",
      full_name: "Marta Alvarez de Sotomayor-Whitfield",
      email: "marta.alvarez.de.sotomayor.whitfield@brandt-automotive.example",
      delivery_state: "consumed",
      download_count: 1,
      documents_downloaded: [
        "A document whose title is long enough to have to wrap rather than truncate",
      ],
    }),
  ]),
};

/**
 * A seat that may read the room and not change who is in it. The card keeps its
 * place and loses the invite verb — withholding the page's one explanation
 * would be the defect, withholding twelve controls individually would be noise.
 */
export const ReadOnly: Story = {
  render: access(
    [participant({ download_count: 3, documents_downloaded: ["Terms v4"] })],
    false,
  ),
};

/** At 390px the row's name, its state and its reading line stack instead of
 *  sharing a line with the verbs. */
export const Phone: Story = {
  tags: ["uat-phone"],
  render: access([
    participant({ download_count: 1204, documents_downloaded: ["Terms v4"] }),
  ]),
};
