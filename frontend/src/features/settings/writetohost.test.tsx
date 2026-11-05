/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ContactLink } from "../../design-system/contactlink";
import { LocaleProvider } from "../../i18n";
import { WriteToHost } from "./writetohost";

// An address pressed anywhere under the shell opens the product's composer on
// the record the address belongs to, with the address already in the To line.
// That it is the composer and not the reader's mail client is the whole point:
// a `mailto:` sent the message around the product.

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// The reader's own connected mailbox, which is what the composer sends from.
const CONNECTED_MAILBOX = {
  data: [{ id: "g1", provider: "gmail", status: "connected", scopes: [] }],
};

function stubRoutes(connectors: unknown = CONNECTED_MAILBOX) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) =>
      new URL(request.url).pathname.endsWith("/connectors")
        ? jsonResponse(connectors)
        : jsonResponse({ data: [] }),
    ),
  );
}

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <WriteToHost>{ui}</WriteToHost>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("WriteToHost", () => {
  it("opens the composer on the pressed address's record, addressed to it", async () => {
    stubRoutes();
    render(
      <ContactLink
        kind="email"
        value="dung.ly@newsky.example"
        record={{ entityType: "lead", entityId: "l-1" }}
      />,
    );
    expect(screen.queryByRole("dialog")).toBeNull();

    // A button once the roster has answered that there is a mailbox to send
    // from; until then the address is the reader's own client's.
    await userEvent.click(
      await screen.findByRole("button", { name: "dung.ly@newsky.example" }),
    );

    const dialog = await screen.findByRole("dialog", {
      name: /New email/,
    });
    // The To line carries the pressed address as a token — the one the
    // reader can take out again — rather than an empty field to retype it in.
    expect(
      await within(dialog).findByRole("button", {
        name: "Remove dung.ly@newsky.example",
      }),
    ).toBeTruthy();
  });

  it("hands the address to the reader's own client when no mailbox is connected", async () => {
    // A calendar is a connection and not a mailbox: the composer would have
    // nothing to send from, so the address is a `mailto:` and no drawer opens.
    stubRoutes({
      data: [{ id: "c1", provider: "gcal", status: "connected", scopes: [] }],
    });
    render(
      <ContactLink
        kind="email"
        value="dung.ly@newsky.example"
        record={{ entityType: "lead", entityId: "l-1" }}
      />,
    );

    const link = await screen.findByRole("link", {
      name: "dung.ly@newsky.example",
    });
    expect(link.getAttribute("href")).toBe("mailto:dung.ly@newsky.example");
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.queryByRole("dialog")).toBeNull();
  });

  it("closes the composer and leaves the address pressable again", async () => {
    stubRoutes();
    render(
      <ContactLink
        kind="email"
        value="dung.ly@newsky.example"
        record={{ entityType: "lead", entityId: "l-1" }}
      />,
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "dung.ly@newsky.example" }),
    );
    await screen.findByRole("dialog", { name: /New email/ });

    await userEvent.click(screen.getByRole("button", { name: "Close" }));

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(
      screen.getByRole("button", { name: "dung.ly@newsky.example" }),
    ).toBeTruthy();
  });

  it("drafts on arrival when an AI verb opened it, steered by the verb's words", async () => {
    // A writer's voice to draft in; without one the composer waits for a press.
    const voice = {
      data: [{ id: "vp-1", status: "ready", maturity: "provisional" }],
      page: { next_cursor: null, has_more: false },
    };
    const drafts: unknown[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        const path = new URL(request.url).pathname;
        if (path.endsWith("/connectors"))
          return jsonResponse(CONNECTED_MAILBOX);
        if (path.endsWith("/voice-profiles")) return jsonResponse(voice);
        if (
          request.method === "POST" &&
          path.endsWith("/contacts/c-1/draft-email")
        ) {
          drafts.push(await request.clone().json());
        }
        return jsonResponse({ data: [] });
      }),
    );
    render(
      <ContactLink
        kind="email"
        appearance="button"
        label="Email"
        draft={{ label: "Draft email", intent: "deliver on a commitment" }}
        address="alice@example.com"
        record={{ entityType: "contact", entityId: "c-1" }}
      />,
    );

    await userEvent.click(
      await screen.findByRole("button", { name: "Draft email" }),
    );

    await waitFor(() => expect(drafts).toHaveLength(1));
    expect(drafts[0]).toMatchObject({ intent: "deliver on a commitment" });
  });
});
