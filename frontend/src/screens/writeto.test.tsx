/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { ContactLink } from "../design-system/contactlink";
import { LocaleProvider } from "../i18n";
import { WriteToHost } from "./writeto";

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

function stubRoutes() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => jsonResponse({ data: [] })),
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

    await userEvent.click(
      screen.getByRole("button", { name: "dung.ly@newsky.example" }),
    );

    const dialog = await screen.findByRole("dialog", { name: /Draft email/ });
    // The To line carries the pressed address as a token — the one the
    // reader can take out again — rather than an empty field to retype it in.
    expect(
      await within(dialog).findByRole("button", {
        name: "Remove dung.ly@newsky.example",
      }),
    ).toBeTruthy();
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
      screen.getByRole("button", { name: "dung.ly@newsky.example" }),
    );
    await screen.findByRole("dialog", { name: /Draft email/ });

    await userEvent.click(screen.getByRole("button", { name: "Cancel" }));

    expect(screen.queryByRole("dialog")).toBeNull();
    expect(
      screen.getByRole("button", { name: "dung.ly@newsky.example" }),
    ).toBeTruthy();
  });
});
