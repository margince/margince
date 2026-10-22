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
import { LocaleProvider } from "../../i18n";
import { INSTALLATION_SETTINGS_KEY } from "../../lib/uploadlimit";
import { DealsScreen } from "./deals";

// The New deal form opens on the installation's base currency and offers the
// rate sheet's currencies right after it. NOK and SEK are not among the four
// fallback codes, so the old fixed list cannot pass these cases.

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

function stubApi(posted: unknown[]) {
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
    const currency = await screen.findByLabelText("Currency *");
    expect(currency.textContent).toContain("NOK");
    await user.click(currency);
    await waitFor(() =>
      expect(
        within(screen.getByRole("listbox"))
          .getAllByRole("option")
          .slice(0, 2)
          .map((option) => option.textContent),
      ).toEqual(["NOK", "SEK"]),
    );
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
