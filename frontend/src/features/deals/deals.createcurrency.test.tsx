/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

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
import { pickOption } from "../../design-system/select-testing";
import { LocaleProvider } from "../../i18n";
import { INSTALLATION_SETTINGS_KEY } from "../../lib/uploadlimit";
import { meFixture } from "../../testing/mefixture";
import { DealsScreen } from "./deals";

// The New deal form opens on the installation's base currency and offers the
// rate sheet's currencies right after it. NOK and SEK are not among the four
// fallback codes, so the old fixed list cannot pass these cases. It also asks
// who owns the deal, where it came from and which partner brought it.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

const pipeline = {
  id: "pl",
  name: "Sales",
  is_default: true,
  position: 0,
  stages: [
    {
      id: "s1",
      pipeline_id: "pl",
      name: "Qualify",
      position: 1,
      semantic: "open",
      win_probability: 20,
    },
  ],
};

const emptyPage = { data: [], page: { next_cursor: null } };

const settings = {
  company_name: "Fjord AS",
  base_currency: "NOK",
  base_currency_locked: false,
  max_upload_bytes: 26_214_400,
};

const ME = meFixture({
  allow: { deal: ["create", "read"], company: ["read"], user: ["read"] },
});

function stubApi(posted: unknown[], withPartner = false) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const request =
        input instanceof Request ? input : new Request(String(input), init);
      const path = new URL(request.url, "https://test.local").pathname.replace(
        /^\/v1/,
        "",
      );
      if (request.method === "POST" && path === "/deals") {
        posted.push(await request.json());
        return jsonResponse({ id: "d-new", name: "x", version: 1 }, 201);
      }
      if (path === "/installation/settings") {
        return jsonResponse(settings);
      }
      if (path === "/fx-rates/currencies") {
        return jsonResponse({ base_currency: "NOK", data: ["NOK", "SEK"] });
      }
      if (path === "/pipelines") {
        return jsonResponse({ data: [pipeline], page: { next_cursor: null } });
      }
      if (path === "/me") {
        return jsonResponse(ME);
      }
      if (path === "/users") {
        return jsonResponse({
          data: [
            { id: ME.user.id, display_name: "Me Myself", is_agent: false },
            { id: "u-anna", display_name: "Anna Weber", is_agent: false },
          ],
          page: { next_cursor: null },
        });
      }
      if (path === "/acquisition-sources") {
        return jsonResponse({
          data: [
            {
              id: "src-1",
              key: "referral",
              label: "Referral",
              sort_order: 1,
              active: true,
            },
          ],
        });
      }
      if (path === "/partners" && withPartner) {
        return jsonResponse({
          data: [{ company_id: "co-x" }],
          page: { next_cursor: null },
        });
      }
      if (path === "/companies" && withPartner) {
        return jsonResponse({
          data: [{ id: "co-x", display_name: "Xentral" }],
          page: { next_cursor: null },
        });
      }
      return jsonResponse(emptyPage);
    }),
  );
}

// The app shows its splash until the installation settings have answered, so
// no form opens before the base is known. The seeded query stands for that.
function renderDeals() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(INSTALLATION_SETTINGS_KEY, settings);
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <DealsScreen startCreating />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("the New deal currency", () => {
  it("opens on the base and lists the rate sheet's currencies next", async () => {
    stubApi([]);
    renderDeals();
    const user = userEvent.setup();
    const dialog = within(await screen.findByRole("dialog"));
    // The base stands picked, named in the reader's language.
    expect(await dialog.findByText("NOK – Norwegian Krone")).toBeTruthy();

    await user.type(dialog.getByLabelText("Currency *"), "kr");
    await waitFor(() =>
      expect(
        dialog
          .getAllByRole("button", { name: /Kr/ })
          .slice(0, 2)
          .map((option) => option.textContent),
      ).toEqual(["NOK – Norwegian Krone", "SEK – Swedish Krona"]),
    );
  });

  it("finds a currency by its name", async () => {
    stubApi([]);
    renderDeals();
    const user = userEvent.setup();
    const dialog = within(await screen.findByRole("dialog"));

    await user.type(await dialog.findByLabelText("Currency *"), "dong");
    await user.click(
      await dialog.findByRole("button", { name: "VND – Vietnamese Dong" }),
    );

    expect(dialog.getByText("VND – Vietnamese Dong")).toBeTruthy();
  });

  it("sends the owner, the source and the partner it was given", async () => {
    const posted: unknown[] = [];
    stubApi(posted, true);
    renderDeals();
    const user = userEvent.setup();
    const dialog = within(await screen.findByRole("dialog"));
    await user.type(dialog.getByLabelText("Deal name *"), "Fjord rollout");

    await pickOption(user, dialog.getByLabelText("Owner"), "Anna Weber");
    await pickOption(
      user,
      dialog.getByLabelText("Acquisition source"),
      "Referral",
    );
    await waitFor(() =>
      expect(dialog.getByLabelText("Referring partner")).toBeTruthy(),
    );
    await pickOption(
      user,
      dialog.getByLabelText("Referring partner"),
      "Xentral",
    );
    await user.click(dialog.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({
      owner_id: "u-anna",
      acquisition_source: "referral",
      partner_company_id: "co-x",
    });
  });

  it("makes the reader the owner when the owner is left alone", async () => {
    const posted: unknown[] = [];
    stubApi(posted);
    renderDeals();
    const user = userEvent.setup();
    const dialog = within(await screen.findByRole("dialog"));
    await user.type(dialog.getByLabelText("Deal name *"), "Fjord rollout");
    await user.click(dialog.getByRole("button", { name: "Create" }));

    await waitFor(() => expect(posted).toHaveLength(1));
    // Null: the server makes the creator the owner.
    expect(posted[0]).toMatchObject({ owner_id: null });
  });

  it("sends the base it showed when the reader leaves the currency alone", async () => {
    const posted: unknown[] = [];
    stubApi(posted);
    renderDeals();
    const user = userEvent.setup();
    const dialog = within(await screen.findByRole("dialog"));
    await user.type(dialog.getByLabelText("Deal name *"), "Fjord rollout");
    await user.type(dialog.getByLabelText("Value"), "480");
    await user.click(dialog.getByRole("button", { name: "Create" }));
    await waitFor(() => expect(posted).toHaveLength(1));
    expect(posted[0]).toMatchObject({ currency: "NOK", amount_minor: 48_000 });
  });
});
