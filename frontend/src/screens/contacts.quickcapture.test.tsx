/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

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
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { ContactsScreen } from "./contacts";

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

const emptyPage = { data: [], page: { next_cursor: null, has_more: false } };

// Two companies share a name on purpose: the pick has to name WHICH one, and a
// name the server resolved would have no way to.
const companies = [
  { id: "co-acme", display_name: "Acme GmbH" },
  { id: "co-acme-2", display_name: "Acme GmbH" },
];

// Answers the list screen's reads, searches companies by the typed name, and
// records the one quick-capture write the form sends.
function stubQuickCaptureApi() {
  const posted: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const url = new URL(request.url);
      if (
        request.method === "POST" &&
        url.pathname.endsWith("/quick-capture")
      ) {
        posted.push(JSON.parse(await request.text()));
        return jsonResponse(
          { contact: { id: "p-new", full_name: "Dana Buyer", version: 1 } },
          201,
        );
      }
      if (url.pathname.endsWith("/companies")) {
        const q = (url.searchParams.get("q") ?? "").toLowerCase();
        return jsonResponse({
          data: companies.filter((company) =>
            company.display_name.toLowerCase().includes(q),
          ),
          page: { next_cursor: null, has_more: false },
        });
      }
      if (url.pathname.endsWith("/me")) {
        return jsonResponse(
          meFixture({ allow: { contact: ["read", "create"] } }),
        );
      }
      return jsonResponse(emptyPage);
    }),
  );
  return posted;
}

async function openQuickCapture() {
  const user = userEvent.setup();
  render(
    <QueryClientProvider
      client={
        new QueryClient({ defaultOptions: { queries: { retry: false } } })
      }
    >
      <LocaleProvider initial="en">
        <ContactsScreen />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  await user.click(screen.getByTestId("quick-capture"));
  await user.type(screen.getByLabelText("Full name *"), "Dana Buyer");
  return user;
}

describe("Quick capture's company box", () => {
  it("attaches the existing company the reader picks, by id", async () => {
    const posted = stubQuickCaptureApi();
    const user = await openQuickCapture();

    const box = screen.getByRole("combobox", { name: "Company" });
    await user.type(box, "Acme");
    const offers = await within(
      await screen.findByRole("listbox"),
    ).findAllByRole("option", { name: "Acme GmbH" });
    await user.click(offers[1]);

    expect(box).toHaveValue("Acme GmbH");
    expect(
      screen.getByText("Adds the contact to this existing company."),
    ).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({
      full_name: "Dana Buyer",
      company_id: "co-acme-2",
    });
    expect(posted[0]).not.toHaveProperty("company_name");
  });

  it("creates a company from a name nobody picked", async () => {
    const posted = stubQuickCaptureApi();
    const user = await openQuickCapture();

    await user.type(
      screen.getByRole("combobox", { name: "Company" }),
      "Nordwand AG",
    );
    expect(
      screen.getByText(
        "Creates a new company unless one is picked from the list.",
      ),
    ).toBeTruthy();
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({ company_name: "Nordwand AG" });
    expect(posted[0]).not.toHaveProperty("company_id");
  });

  it("drops the picked company once the reader edits its name", async () => {
    const posted = stubQuickCaptureApi();
    const user = await openQuickCapture();

    const box = screen.getByRole("combobox", { name: "Company" });
    await user.type(box, "Acme");
    const [first] = await within(
      await screen.findByRole("listbox"),
    ).findAllByRole("option", { name: "Acme GmbH" });
    await user.click(first);
    await user.type(box, " Holding");
    await user.click(screen.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({ company_name: "Acme GmbH Holding" });
    expect(posted[0]).not.toHaveProperty("company_id");
  });
});
