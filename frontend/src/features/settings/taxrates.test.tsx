/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { en } from "../../i18n/en";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { jsonResponse, render } from "../../testing/settings.testkit";
import { TaxRatesCard } from "./taxrates";

// Settings › Company › Tax rates: an admin reads the sheet and records a rate;
// a seat without the read grant is told the sheet is withheld.

const KEEPER: GrantSpec = { tax_rate: ["read", "create"] };
const READER: GrantSpec = { tax_rate: ["read"] };

const RATE = {
  id: "tr-1",
  country: "de",
  category: "standard",
  rate: "19.00",
  valid_from: "2026-01-01",
  created_at: "2026-01-01T08:00:00Z",
};

type Call = { method: string; url: string; body: unknown };

function backend(allow: GrantSpec, calls: Call[] = []) {
  return vi.fn(async (request: Request) => {
    const raw = request.method === "GET" ? "" : await request.text();
    calls.push({
      method: request.method,
      url: request.url,
      body: raw ? JSON.parse(raw) : undefined,
    });
    if (request.url.endsWith("/me")) {
      return jsonResponse(meFixture({ allow }));
    }
    if (request.method === "POST") {
      return jsonResponse({ rate: RATE, classified_products: 3 }, 201);
    }
    return jsonResponse({ data: [RATE] });
  });
}

beforeEach(() => {
  localStorage.setItem("margince.workspaceSlug", "acme");
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  localStorage.clear();
});

it("lists the rates and offers no editor to a seat that only reads them", async () => {
  vi.stubGlobal("fetch", backend(READER));
  render(<TaxRatesCard />);
  expect(await screen.findByText("19.00 %")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: en["taxRates.add"] })).toBeNull();
});

it("records a rate in the sheet's spelling", async () => {
  const user = userEvent.setup();
  const calls: Call[] = [];
  vi.stubGlobal("fetch", backend(KEEPER, calls));
  render(<TaxRatesCard />);
  await user.click(
    await screen.findByRole("button", { name: en["taxRates.add"] }),
  );
  const dialog = await screen.findByRole("dialog");
  await pickOption(
    user,
    within(dialog).getByRole("combobox", { name: en["taxRates.colCountry"] }),
    "Austria",
  );
  await pickOption(
    user,
    within(dialog).getByRole("combobox", { name: en["taxRates.colCategory"] }),
    en["taxRates.category.reduced"],
  );
  await user.type(
    within(dialog).getByRole("textbox", { name: en["taxRates.colRate"] }),
    "10,5",
  );
  await user.click(
    within(dialog).getByRole("button", { name: en["taxRates.save"] }),
  );
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  const post = calls.find((call) => call.method === "POST");
  expect(post?.body).toMatchObject({
    country: "at",
    category: "reduced",
    rate: "10.5",
  });
});

it("says the sheet is withheld from a seat without the read grant", async () => {
  vi.stubGlobal("fetch", backend({}));
  render(<TaxRatesCard />);
  expect(await screen.findByText(en["taxRates.withheld"])).toBeInTheDocument();
});
