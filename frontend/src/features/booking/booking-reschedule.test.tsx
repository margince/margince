/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { formatDateTime } from "../../format/format";
import {
  browserZone,
  startOfDayInZone,
  wallClockIntent,
} from "../../format/timezone";
import { LocaleProvider } from "../../i18n";
import { BookingReschedule } from "./booking-reschedule";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

// Every window the guest asked for, as the `from` of each availability read.
function stubAvailability(
  slots: readonly { start: string; end: string }[] = [],
): string[] {
  const asked: string[] = [];
  vi.stubGlobal("fetch", async (request: Request) => {
    asked.push(new URL(request.url).searchParams.get("from") ?? "");
    return new Response(JSON.stringify({ slots, truncated: false }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  });
  return asked;
}

function renderReschedule(onSelect: (slot: unknown) => void = () => undefined) {
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <BookingReschedule token="guest" pending={false} onSelect={onSelect} />
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
    expect(asked.at(-1)).toBe(startOfDayInZone("2026-10-08", browserZone())),
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

it("sends the chosen time with the local clock and zone it was picked in", async () => {
  const slot = { start: "2026-10-08T08:00:00Z", end: "2026-10-08T08:30:00Z" };
  stubAvailability([slot]);
  const chosen: unknown[] = [];
  const user = userEvent.setup();
  renderReschedule((picked) => chosen.push(picked));

  await user.click(
    await screen.findByRole("button", {
      name: formatDateTime(slot.start, "en", browserZone()),
    }),
  );
  await user.click(screen.getByRole("button", { name: "Confirm new time" }));

  expect(chosen).toEqual([
    { ...slot, start_intent: wallClockIntent(slot.start, browserZone()) },
  ]);
});
