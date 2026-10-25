/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { startOfDayInZone, viewerZone } from "../../format/timezone";
import { LocaleProvider } from "../../i18n";
import { BookingReschedule } from "./booking-reschedule";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

// Every window the guest asked for, as the `from` of each availability read.
function stubAvailability(): string[] {
  const asked: string[] = [];
  vi.stubGlobal("fetch", async (request: Request) => {
    asked.push(new URL(request.url).searchParams.get("from") ?? "");
    return new Response(JSON.stringify({ slots: [], truncated: false }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  });
  return asked;
}

function renderReschedule() {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <BookingReschedule
          token="guest"
          pending={false}
          onSelect={() => undefined}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const startingDate = () =>
  screen.getByRole<HTMLInputElement>("textbox", { name: "Starting date" });

it("asks for the times from the start of the day picked on the calendar", async () => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-10-05T08:00:00Z"));
  const asked = stubAvailability();
  const user = userEvent.setup();
  renderReschedule();

  await user.click(screen.getByRole("button", { name: /^Choose date/ }));
  await user.click(
    await screen.findByRole("button", { name: "Thursday, 8 October 2026" }),
  );

  await waitFor(() =>
    expect(asked.at(-1)).toBe(startOfDayInZone("2026-10-08", viewerZone())),
  );
  expect(startingDate().value).toBe("08/10/2026");
});

it("keeps the day it shows when the box is cleared", async () => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-10-05T08:00:00Z"));
  const asked = stubAvailability();
  const user = userEvent.setup();
  renderReschedule();
  await waitFor(() => expect(asked).toHaveLength(1));
  const shown = startingDate().value;

  await user.click(screen.getByRole("button", { name: /^Choose date/ }));
  await user.click(await screen.findByRole("button", { name: "Clear" }));

  expect(startingDate().value).toBe(shown);
  expect(shown).not.toBe("");
  expect(asked).toHaveLength(1);
});
