/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import { bookingProfile } from "../../testing/book.testkit";
import { installFetchStub, jsonResponse } from "../../testing/story-utils";
import { BookingGuestScreen } from "./booking-guest";

const OFFERED = [30, 45, 60];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.history.replaceState(null, "", "/");
});

// The server's answer: the asked length when the page offers it, else the
// default. It records every length availability was asked for.
function mountGuest(address: string) {
  globalThis.history.replaceState(null, "", address);
  const asked: (string | null)[] = [];
  installFetchStub({
    "GET /public/booking/host/profile": (_body, url) => {
      const length = Number(url.searchParams.get("l"));
      return jsonResponse({
        ...bookingProfile,
        paused: false,
        offered_lengths: OFFERED,
        duration_minutes: OFFERED.includes(length) ? length : 30,
      });
    },
    "GET /public/booking/host/availability": (_body, url) => {
      asked.push(url.searchParams.get("l"));
      return jsonResponse({ slots: [], truncated: false });
    },
  });
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <BookingGuestScreen hostSlug="host" />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return asked;
}

const pressed = (name: string) =>
  screen.getByRole("button", { name }).getAttribute("aria-pressed");

it("asks for the times of the length the guest picks and keeps it in the address", async () => {
  const asked = mountGuest("/book/host");
  await userEvent.click(await screen.findByRole("button", { name: "45 min" }));
  await waitFor(() => expect(asked).toContain("45"));
  expect(new URLSearchParams(globalThis.location.search).get("l")).toBe("45");
  await waitFor(() => expect(pressed("45 min")).toBe("true"));
  expect(pressed("30 min")).toBe("false");
});

it.each([
  ["opens at the length a link asks for", "/book/host?l=60", "60 min"],
  [
    "falls back to the default for a length not offered",
    "/book/host?l=47",
    "30 min",
  ],
])("%s", async (_name, address, chosen) => {
  mountGuest(address);
  await waitFor(() => expect(pressed(chosen)).toBe("true"));
  for (const length of OFFERED.map((minutes) => `${minutes} min`))
    if (length !== chosen) expect(pressed(length)).toBe("false");
});
