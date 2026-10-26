// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../../i18n";
import { meFixture } from "../../../testing/mefixture";
import { CompanyContactsList } from "./contacts";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// Records every write, and answers the company's contact list and the session.
function stubApi() {
  const writes: { path: string; body: unknown }[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url);
      if (request.method === "POST") {
        writes.push({
          path: url.pathname,
          body: JSON.parse(await request.text()),
        });
        return jsonResponse(
          { contact: { id: "p-new", full_name: "Dana Buyer", version: 1 } },
          201,
        );
      }
      if (url.pathname.endsWith("/me")) {
        return jsonResponse(
          meFixture({ allow: { contact: ["read", "create"] } }),
        );
      }
      return jsonResponse({
        data: [],
        page: { next_cursor: null, has_more: false },
      });
    }),
  );
  return writes;
}

function providers(ui: ReactNode, client: QueryClient) {
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>
  );
}

function newClient() {
  return new QueryClient({ defaultOptions: { queries: { retry: false } } });
}

describe("Creating a contact from a company's Contacts tab", () => {
  it("offers quick capture alone, filed under the company", async () => {
    const writes = stubApi();
    render(
      providers(
        <CompanyContactsList companyId="co-acme" mayCreate />,
        newClient(),
      ),
    );
    const user = userEvent.setup();

    expect(screen.queryByRole("button", { name: "New contact" })).toBeNull();
    expect(screen.queryByTestId("vcard-import")).toBeNull();
    await user.click(screen.getByTestId("quick-capture"));
    await user.type(await screen.findByLabelText("Full name *"), "Dana Buyer");
    expect(screen.queryByRole("combobox", { name: "Company" })).toBeNull();
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(writes).toHaveLength(1));
    expect(writes[0]).toMatchObject({
      path: expect.stringMatching(/\/contacts\/quick-capture$/),
      body: { full_name: "Dana Buyer", company_id: "co-acme" },
    });
    expect(writes[0]?.body).not.toHaveProperty("company_name");
  });

  it("offers no create to a reader who may not write the company", () => {
    stubApi();
    render(providers(<CompanyContactsList companyId="co-acme" />, newClient()));
    expect(screen.queryByTestId("quick-capture")).toBeNull();
  });

  it("closes an open form when the reader moves to another company", async () => {
    const writes = stubApi();
    const client = newClient();
    const { rerender } = render(
      providers(<CompanyContactsList companyId="co-acme" mayCreate />, client),
    );
    const user = userEvent.setup();
    await user.click(screen.getByTestId("quick-capture"));
    await user.type(await screen.findByLabelText("Full name *"), "Dana Buyer");

    rerender(
      providers(<CompanyContactsList companyId="co-other" mayCreate />, client),
    );
    // The form the reader opened on the first company closes with it.
    await waitFor(() =>
      expect(screen.queryByLabelText("Full name *")).toBeNull(),
    );
    expect(writes).toHaveLength(0);
  });
});
