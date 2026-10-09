// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  bookingInvitation,
  bookingProfile,
} from "../../src/screens/book.testkit";
import { publicSlots } from "./fixtures";
import type { Handler } from "./server";

export const schedulingProfile = {
  ...bookingProfile,
  slug: "host-1",
  public_url: "https://crm.example.test/#/book/host-1",
};

export const calendars = [
  { id: "primary", name: "Work calendar", writable: true, primary: true },
];

export const guestBooking = { ...bookingInvitation, ...publicSlots[0] };

export const hostAvailability = { slots: publicSlots };

// The public door demands what the real one does and no more: the consent
// wording shown and its version. The anonymous page cannot learn a purpose
// id, and the server resolves its own lane.
export const bookSlot: Handler = ({ route, json }) => {
  const body = route.request().postDataJSON();
  if (!body?.consent?.policy_version || !body?.consent?.wording) {
    return json(
      {
        title: "Unprocessable",
        detail: "consent is mandatory on the public capture surface",
      },
      422,
    );
  }
  if (body.start === publicSlots[1].start) {
    return json(
      {
        title: "Conflict",
        detail: "slot no longer available",
        code: "slot_taken",
      },
      409,
    );
  }
  return json(
    {
      start: body.start,
      end: body.end,
      booking: "pending",
      invitation: {
        ...bookingInvitation,
        start: body.start,
        end: body.end,
        management_token: "guest-booking",
      },
    },
    201,
  );
};

export const availability = {
  slots: [
    { start: "2026-07-06T09:00:00Z", end: "2026-07-06T09:30:00Z" },
    { start: "2026-07-06T10:00:00Z", end: "2026-07-06T10:30:00Z" },
  ],
};
