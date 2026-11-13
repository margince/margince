// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

// The Shared mailboxes card in each state an admin or a colleague meets it. The
// last case is the write that moves a mailbox's whole history.

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { SharedMailboxesCard } from "./sharedmailboxes";

type SharedMailbox = components["schemas"]["SharedMailbox"];

const SALES_TEAM = "01a05500-0000-7000-8000-0000000000e1";
const SUPPORT_TEAM = "01a05500-0000-7000-8000-0000000000e2";

const SALES: SharedMailbox = {
  id: "01a05500-0000-7000-8000-0000000000f1",
  provider: "imap",
  address: "sales@demo.test",
  reader_team_id: SALES_TEAM,
  mail_visibility: "shared",
  status: "connected",
  connected_by: null,
  connected_at: "2026-11-12T09:00:00Z",
};

type Sent = { method: string; path: string; body: unknown };

function json(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

/** Answers the list with `list`, and records every write the card sends. */
function backend(list: () => Response): Sent[] {
  const sent: Sent[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname;
      if (request.method === "GET" && path.endsWith("/shared-mailboxes")) {
        return list();
      }
      if (request.method === "PATCH") {
        sent.push({ method: "PATCH", path, body: await request.json() });
        return json({ ...SALES, mail_visibility: "held" });
      }
      throw new Error(`unexpected request: ${request.method} ${path}`);
    }),
  );
  return sent;
}

function draw() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(["teams"], {
    entries: [
      { id: SALES_TEAM, name: "Sales desk" },
      { id: SUPPORT_TEAM, name: "Support" },
    ],
    partial: false,
  });
  return render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <SharedMailboxesCard />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("the Shared mailboxes card", () => {
  it("invites an admin to connect the first one", async () => {
    backend(() => json({ items: [] }));
    draw();

    expect(await screen.findByText("No shared mailboxes yet")).toBeTruthy();
    expect(
      screen.getByRole("button", { name: /Connect a shared mailbox/ }),
    ).toBeTruthy();
  });

  it("lists a connected mailbox with its reader team and visibility", async () => {
    backend(() => json({ items: [SALES] }));
    draw();

    const row = await screen.findByTestId("shared-mailbox");
    expect(within(row).getByText("sales@demo.test")).toBeTruthy();
    expect(within(row).getByText("Capturing")).toBeTruthy();
    expect(within(row).getByText("Read by Sales desk · Shared")).toBeTruthy();
  });

  it("says when a mailbox stopped syncing", async () => {
    backend(() => json({ items: [{ ...SALES, status: "error" }] }));
    draw();

    const row = await screen.findByTestId("shared-mailbox");
    expect(within(row).getByText("Sync error")).toBeTruthy();
  });

  it("tells a colleague without the admin role why there is nothing to manage", async () => {
    backend(() => json({ code: "permission_denied" }, 403));
    draw();

    expect(
      await screen.findByText("Only admins manage shared mailboxes"),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: /Connect a shared mailbox/ }),
    ).toBeNull();
  });

  it("says a change applies to captured mail, and sends it confirmed", async () => {
    const sent = backend(() => json({ items: [SALES] }));
    const user = userEvent.setup();
    draw();

    await user.click(await screen.findByRole("button", { name: "Change" }));
    expect(
      screen.getByText(
        "This applies to all mail the mailbox already captured, not only to new mail.",
      ),
    ).toBeTruthy();
    await user.click(screen.getByRole("radio", { name: /Held/ }));
    await user.click(
      screen.getByRole("checkbox", { name: "Also hold mail already captured" }),
    );
    await user.click(screen.getByRole("button", { name: "Apply to all mail" }));

    await vi.waitFor(() => expect(sent).toHaveLength(1));
    expect(sent[0]).toEqual({
      method: "PATCH",
      path: `/v1/shared-mailboxes/${SALES.id}`,
      body: { confirm: true, mail_visibility: "held", apply_to_history: true },
    });
  });
});
