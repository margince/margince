/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { de } from "../i18n/de";
import { bookingProfile } from "./book.testkit";
import { BookingGuestScreen } from "./booking-guest";
import { BookingReschedule } from "./booking-reschedule";
import { installFetchStub, jsonResponse } from "./story-utils";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it.each(["public", "reschedule"])(
  "explains a %s booking horizon in the guest's language",
  async (surface) => {
    const failure = {
      status: 422,
      code: "validation_error",
      detail: "English server explanation",
      details: {
        errors: [
          {
            field: "from",
            code: "booking_horizon",
            message: "English server explanation",
          },
        ],
      },
    };
    installFetchStub({
      "GET /public/booking/host/profile": () => jsonResponse(bookingProfile),
      "GET /public/booking/host/availability": () => jsonResponse(failure, 422),
      "GET /public/meeting/guest/availability": () =>
        jsonResponse(failure, 422),
    });
    render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <LocaleProvider initial="de">
          {surface === "public" ? (
            <BookingGuestScreen hostSlug="host" />
          ) : (
            <BookingReschedule
              token="guest"
              pending={false}
              onSelect={() => undefined}
            />
          )}
        </LocaleProvider>
      </QueryClientProvider>,
    );
    expect(
      await screen.findByText(de["scheduling.windowHorizon"]),
    ).toBeTruthy();
    expect(screen.queryByText("English server explanation")).toBeNull();
    expect(screen.getByLabelText(de["scheduling.date"])).toBeTruthy();
  },
);
