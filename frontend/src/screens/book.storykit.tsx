import type { ReactNode } from "react";
import { BookingScreen } from "./book";
import {
  bookingConnection,
  bookingContact,
  bookingInvitation,
  bookingProfile,
  bookingSlots,
} from "./book.testkit";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "./story-utils";
export function bookingFrame(view: () => ReactNode, overrides: RouteMap = {}) {
  return () => {
    installFetchStub({
      "GET /connectors": () => jsonResponse({ data: [bookingConnection] }),
      "GET /scheduling/profile": () => jsonResponse(bookingProfile),
      "GET /scheduling/calendars": () =>
        jsonResponse([
          {
            id: "primary",
            name: "Work calendar",
            primary: true,
            writable: true,
          },
          {
            id: "personal",
            name: "Personal calendar",
            primary: false,
            writable: false,
          },
        ]),
      "GET /public/booking/ada-lovelace/profile": () =>
        jsonResponse(bookingProfile),
      "GET /public/booking/ada-lovelace/availability": () =>
        jsonResponse({ slots: bookingSlots, truncated: false }),
      "GET /availability": () =>
        jsonResponse({ slots: bookingSlots, truncated: false }),
      [`GET /contacts/${bookingContact.id}`]: () =>
        jsonResponse(bookingContact),
      "GET /public/meeting/private-link": () => jsonResponse(bookingInvitation),
      "GET /public/proposal/personal-link": () =>
        jsonResponse({
          profile: {
            ...bookingProfile,
            title: "Project discovery",
            location: "Video call",
            duration_minutes: 30,
          },
          description: "Let's explore the scope and next steps.",
          options: bookingSlots,
          expires_at: "2026-10-06T09:00:00Z",
          used: false,
        }),
      "GET /public/proposal/personal-link/availability": () =>
        jsonResponse({ slots: bookingSlots, truncated: false }),
      ...overrides,
    });
    return <StoryProviders>{view()}</StoryProviders>;
  };
}
export function bookingStory(hostSlug?: string, overrides: RouteMap = {}) {
  return bookingFrame(() => <BookingScreen hostSlug={hostSlug} />, overrides);
}
