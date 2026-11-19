// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */

// The Shared mailboxes card in each state an admin or a colleague meets it. The
// last case is the write that moves a mailbox's whole history.

import "@testing-library/jest-dom/vitest";

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { pickOption } from "../../design-system/select-testing";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
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
function backend(
  list: () => Response,
  connect: () => Response = () => json(SALES),
): Sent[] {
  const sent: Sent[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const path = new URL(request.url).pathname;
      if (request.method === "GET" && path.endsWith("/shared-mailboxes")) {
        return list();
      }
      if (request.method === "POST" && path.endsWith("/shared-mailboxes")) {
        sent.push({ method: "POST", path, body: await request.json() });
        return connect();
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

  it("shows a span no sync read, and puts the import that recovers it first", async () => {
    backend(() =>
      json({
        items: [
          {
            ...SALES,
            unread_period: {
              from: "2026-03-01T09:00:00Z",
              to: "2026-03-04T09:00:00Z",
            },
          },
        ],
      }),
    );
    draw();

    const row = await screen.findByTestId("shared-mailbox");
    expect(
      within(row).getByText(/^Mail from .+ was never read\./),
    ).toBeTruthy();
    expect(
      within(row).getByRole("button", { name: "Import older mail" }),
    ).toBeTruthy();
    expect(within(row).queryByRole("button", { name: "Change" })).toBeNull();
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

describe("connecting a shared mailbox", () => {
  async function openConnect() {
    const user = userEvent.setup();
    draw();
    await user.click(
      await screen.findByRole("button", { name: /Connect a shared mailbox/ }),
    );
    const dialog = await screen.findByRole("dialog");
    const box = (label: string) =>
      within(dialog).getByRole("textbox", { name: label });
    return { user, dialog, box };
  }

  async function fillValid(
    user: ReturnType<typeof userEvent.setup>,
    dialog: HTMLElement,
    box: (label: string) => HTMLElement,
  ) {
    await user.type(box(en["sharedMailboxes.address"]), "sales@demo.test");
    await pickOption(
      user,
      within(dialog).getByRole("combobox", {
        name: en["sharedMailboxes.readerTeam"],
      }),
      "Sales desk",
    );
    await user.type(box(en["connectors.imapHost"]), "imap.demo.test");
    await user.type(box(en["connectors.imapUsername"]), "sales@demo.test");
    await user.type(
      within(dialog).getByLabelText(en["connectors.imapSecret"], {
        exact: false,
      }),
      "app-password",
    );
  }

  const press = (
    user: ReturnType<typeof userEvent.setup>,
    dialog: HTMLElement,
  ) =>
    user.click(
      within(dialog).getByRole("button", {
        name: en["sharedMailboxes.connect"],
      }),
    );

  it("names each field it cannot send, on that field, before sending", async () => {
    const sent = backend(() => json({ items: [] }));
    const { user, dialog, box } = await openConnect();
    await user.type(box(en["sharedMailboxes.address"]), "not-an-email");
    const port = box(en["connectors.imapPort"]);
    await user.clear(port);
    await user.type(port, "abc");
    await press(user, dialog);

    expect(
      await within(dialog).findByText(
        "Enter an email address for Mailbox address.",
      ),
    ).toBeVisible();
    expect(
      within(dialog).getByText("Enter a whole number for Port."),
    ).toBeVisible();
    expect(within(dialog).getByText("IMAP server is required.")).toBeVisible();
    expect(
      within(dialog).getByText("Team that reads it is required."),
    ).toBeVisible();
    await waitFor(() =>
      expect(box(en["sharedMailboxes.address"])).toHaveFocus(),
    );
    expect(sent).toHaveLength(0);

    await user.clear(port);
    await user.type(port, "0");
    expect(port).not.toHaveAttribute("aria-invalid", "true");
    await press(user, dialog);
    expect(
      await within(dialog).findByText("Port must be from 1 to 65535."),
    ).toBeVisible();
  });

  it("puts the server's refusal on the field it names, and keeps the password", async () => {
    const sent = backend(
      () => json({ items: [] }),
      () =>
        json(
          {
            title: "Unprocessable Entity",
            status: 422,
            code: "validation_error",
            detail: "`address` must be an email address",
            details: {
              errors: [
                {
                  field: "address",
                  code: "email_malformed",
                  message: "`address` must be an email address",
                },
              ],
            },
          },
          422,
        ),
    );
    const { user, dialog, box } = await openConnect();
    await fillValid(user, dialog, box);
    await press(user, dialog);

    await waitFor(() => expect(sent).toHaveLength(1));
    expect(
      await within(dialog).findAllByText(
        "Enter an email address for Mailbox address.",
      ),
    ).toHaveLength(1);
    expect(box(en["sharedMailboxes.address"])).toHaveAttribute(
      "aria-invalid",
      "true",
    );
    expect(within(dialog).queryByText(/`address`/)).toBeNull();
    expect(
      within(dialog).getByLabelText(en["connectors.imapSecret"], {
        exact: false,
      }),
    ).toHaveValue("app-password");
  });

  it("says a refused login at the foot, as the personal form does", async () => {
    backend(
      () => json({ items: [] }),
      () =>
        json(
          {
            title: "Unprocessable Entity",
            status: 422,
            code: "imap_login_rejected",
            detail: "The mailbox rejected these credentials.",
          },
          422,
        ),
    );
    const { user, dialog, box } = await openConnect();
    await fillValid(user, dialog, box);
    await press(user, dialog);

    expect(
      await within(dialog).findByText(en["connectors.imapLoginRejected"]),
    ).toBeVisible();
  });
});
