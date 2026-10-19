// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { jsonResponse, stubWithSession } from "../../testing/story-utils";
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

const imap: CaptureConnection = {
  ...gmail,
  id: "018f3a1b-0000-7000-8000-0000000000c9",
  provider: "imap",
};

// The roster the card reads, which a test changes to what a connect leaves.
function serve(initial: CaptureConnection[]) {
  const roster = { rows: initial };
  stubWithSession(
    {
      "GET /connectors": () => jsonResponse({ data: roster.rows }),
      "GET /channel-connections": () => jsonResponse({ data: [] }),
      "POST /connectors/imap/connect": () => {
        roster.rows = [imap];
        return jsonResponse(imap);
      },
    },
    {},
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ConnectorsCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { roster, client };
}

// The add verb moves from the empty row to the head once a mailbox arrives. So
// the dialog names where focus lands, because its opener is gone by then.
it("lands on the head's add verb once the empty row that opened the dialog is gone", async () => {
  const user = userEvent.setup();
  const { roster, client } = serve([]);

  await user.click(
    await screen.findByRole("button", { name: "Add connector" }),
  );
  await screen.findByRole("dialog", { name: "Add connector" });
  roster.rows = [gmail];
  await client.invalidateQueries({ queryKey: ["connectors"] });
  await user.keyboard("{Escape}");

  const head = await screen.findByRole("button", { name: "Add connector" });
  await waitFor(() => expect(document.activeElement).toBe(head));
});

it("lands on the head's add verb once an IMAP mailbox connects", async () => {
  const user = userEvent.setup();
  serve([]);

  await user.click(
    await screen.findByRole("button", { name: "Add connector" }),
  );
  await user.click(
    await screen.findByRole("button", { name: "Connect IMAP mailbox" }),
  );
  const form = await screen.findByRole("dialog", {
    name: "Connect IMAP mailbox",
  });
  await user.type(
    screen.getByRole("textbox", { name: /IMAP server/ }),
    "imap.example.com",
  );
  await user.type(
    screen.getByRole("textbox", { name: /Email address/ }),
    "me@example.com",
  );
  await user.type(screen.getByLabelText(/App password/), "secret");
  await user.click(screen.getByRole("button", { name: "Connect" }));

  await waitFor(() => expect(form).not.toBeInTheDocument());
  const head = await screen.findByRole("button", { name: "Add connector" });
  await waitFor(() => expect(document.activeElement).toBe(head));
});
