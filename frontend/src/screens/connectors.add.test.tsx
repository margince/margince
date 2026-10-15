// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { jsonResponse, stubWithSession } from "../testing/story-utils";
import { ConnectorsCard } from "./connectors";

type CaptureConnection = components["schemas"]["CaptureConnection"];

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const gmail: CaptureConnection = {
  id: "018f3a1b-0000-7000-8000-0000000000c1",
  provider: "gmail",
  can_backfill: false,
  status: "connected",
  scopes: [],
  last_synced_at: "2026-07-23T09:30:00Z",
};

const openers: readonly [string, CaptureConnection[]][] = [
  ["the empty roster's row", []],
  ["the panel head", [gmail]],
];

// The add verb moves between the empty row and the panel head as mailboxes
// arrive. So the dialog names where focus lands, not its opener.
it.each(openers)(
  "hands focus back to the add verb in %s when the dialog closes",
  async (_, rows) => {
    stubWithSession(
      {
        "GET /connectors": () => jsonResponse({ data: rows }),
        "GET /channel-connections": () => jsonResponse({ data: [] }),
      },
      {},
    );
    const user = userEvent.setup();
    render(
      <QueryClientProvider
        client={
          new QueryClient({ defaultOptions: { queries: { retry: false } } })
        }
      >
        <LocaleProvider initial="en">
          <ConnectorsCard />
        </LocaleProvider>
      </QueryClientProvider>,
    );

    const verb = await screen.findByRole("button", { name: "Add connector" });
    await user.click(verb);
    await screen.findByRole("dialog", { name: "Add connector" });
    await user.keyboard("{Escape}");

    await waitFor(() => expect(document.activeElement).toBe(verb));
  },
);
