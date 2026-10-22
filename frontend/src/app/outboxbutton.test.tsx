/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { viewerZone } from "../format/timezone";
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { LocaleProvider } from "../i18n";
import { OutboxButton } from "./outboxbutton";

// The clock beside the bell: what is waiting in the sender's own queue, and the
// one door to it from every page.

const base = {
  scheduled_at: "2026-09-01T07:00:00Z",
  scheduled_tz: viewerZone(),
  subject: "The renewal quote",
  to: ["ceo@acme.test"],
  version: 1,
  created_at: "2026-08-20T09:00:00Z",
  updated_at: "2026-08-20T09:00:00Z",
};

function mount(rows: unknown[], status = 200) {
  vi.stubGlobal(
    "fetch",
    vi.fn(
      async () =>
        new Response(JSON.stringify(status === 200 ? rows : {}), {
          status,
          headers: { "Content-Type": "application/json" },
        }),
    ),
  );
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <OutboxButton />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.location.hash = "";
});

it("names how many are waiting and how many are held, so the count is not left to the badge", async () => {
  mount([
    { ...base, id: "a", status: "scheduled" },
    { ...base, id: "b", status: "scheduled" },
    { ...base, id: "c", status: "held" },
    { ...base, id: "d", status: "sent" },
  ]);
  expect(
    await screen.findByRole("button", {
      name: "Scheduled messages, 2 waiting, 1 held",
    }),
  ).toBeInTheDocument();
});

it("draws no figure when nothing is queued, and still offers the way in", async () => {
  mount([{ ...base, id: "d", status: "sent" }]);
  const button = await screen.findByRole("button", {
    name: "Scheduled messages",
  });
  expect(button.textContent).toBe("");
});

it("opens the queue", async () => {
  const user = userEvent.setup();
  mount([{ ...base, id: "a", status: "scheduled" }]);
  await user.click(
    await screen.findByRole("button", {
      name: "Scheduled messages, 1 waiting",
    }),
  );
  expect(globalThis.location.hash).toBe("#/scheduled");
});

it("draws nothing when the queue cannot be read", async () => {
  mount([], 500);
  await vi.waitFor(() => expect(globalThis.fetch).toHaveBeenCalled());
  expect(screen.queryByRole("button")).toBeNull();
});
