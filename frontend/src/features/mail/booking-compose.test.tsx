/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { bookingContact, bookingSlots } from "../../testing/book.testkit";
import { BookingReview } from "../booking/booking-review";
import {
  activity202,
  jsonResponse,
  render,
  stubRoutes,
} from "./compose.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

it("sends the booking email with its preset reason and both contact and deal filing", async () => {
  const user = userEvent.setup();
  const sent = stubRoutes({
    "POST /scheduling/proposals": () =>
      jsonResponse({
        id: "proposal-1",
        url: "https://crm.example.test/book/choice",
        expires_at: "2026-10-06T09:00:00Z",
      }),
    [`GET /contacts/${bookingContact.id}/360`]: () =>
      jsonResponse({
        as_of: "2026-10-01T09:00:00Z",
        contact: bookingContact,
        sections_omitted: [],
        commercial: {
          deal: { deal_id: "d-1", title: "Open negotiation" },
          committee: [],
        },
      }),
    "POST /emails": () => jsonResponse(activity202, 202),
  });
  render(
    <BookingReview
      mode="propose"
      draft={{
        contactId: bookingContact.id,
        attendee: bookingContact.primary_email,
        subject: "Find a meeting time",
        location: "",
        description: "Choose a time to discuss the quote.",
        duration: 30,
        video: true,
        provider: "gcal",
      }}
      onEdit={() => {}}
      picks={bookingSlots}
      onRemovePick={() => {}}
      configured
      zone="Europe/Berlin"
      searchContacts={async () => []}
    />,
  );
  const review = await screen.findByRole("button", {
    name: "Review email · 2 times",
  });
  await waitFor(() => expect(review).toHaveProperty("disabled", false));
  await user.click(review);
  const send = await screen.findByRole("button", { name: "Send" });
  await waitFor(() => expect(send).toHaveProperty("disabled", false));
  await user.click(send);
  await waitFor(() =>
    expect(sent.find((request) => request.key === "POST /emails")).toBeTruthy(),
  );
  expect(
    sent.find((request) => request.key === "POST /emails")?.body,
  ).toMatchObject({
    communication_context: "active_deal_followup",
    links: expect.arrayContaining([
      { entity_type: "contact", entity_id: bookingContact.id },
      { entity_type: "deal", entity_id: "d-1" },
    ]),
  });
});
