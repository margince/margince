/** @vitest-environment happy-dom */
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
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { todayIn } from "../../format/calendarday";
import { formatDate } from "../../format/format";
import { AI_RATE_DAY_ZONE, browserZone } from "../../format/timezone";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { type GrantSpec, meFixture } from "../../testing/mefixture";
import { ModelPricesCard } from "../ai/ai-price-sync";
import { FxRatesCard } from "./rates";

// The reader's zone, real unless a test puts the reader somewhere else.
const viewer = vi.hoisted(() => ({ zone: undefined as string | undefined }));
vi.mock("../../format/timezone", async (importOriginal) => {
  const actual = await importOriginal<typeof import("../../format/timezone")>();
  return { ...actual, browserZone: () => viewer.zone ?? actual.browserZone() };
});

// Each price sheet has three honest states, and a grant picks which one:
// WITHHELD without the object's read grant (fx_rate and ai_model_rate are
// admin/ops-only, so most roles land here), READ-ONLY with the read but no
// upsert — the table plus one caption, write affordances simply absent — and
// EDITABLE with a write verb on a full seat. The server stays the authority
// regardless; these cases pin what the card SAYS.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  vi.useRealTimers();
  viewer.zone = undefined;
});

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

// Setting a rate is an UPSERT: it inserts on a (pair, day) the sheet has never
// carried and replaces the row when it has. The server admits the call on
// either write grant and then demands the specific one inside the transaction,
// so each card asks the same union — hence a `create`-only fixture and an
// `update`-only one both have to open the card they name.
//
// The read rides along in every write fixture because the two are separate
// grants and the card asks for each: a rate-setter who could not read the sheet
// would get the withheld card, and there would be no table to assert against.
const RATE_SETTER: GrantSpec = {
  fx_rate: ["read", "create"],
  ai_model_rate: ["read", "create"],
};

// A run of the price refresh both adds rows and updates them, so it takes both.
const PRICE_ADMIN: GrantSpec = {
  fx_rate: ["read", "create"],
  ai_model_rate: ["read", "create", "update"],
};

// Read on both objects, no write verb anywhere: the read-only state.
const RATE_READER: GrantSpec = {
  fx_rate: ["read"],
  ai_model_rate: ["read"],
};

// The card a heading names, so an affordance can be attributed to ONE sheet.
// Both cards spell "Refresh from sources" identically, and a screen-wide query
// for it cannot tell which card offered it — which is precisely the confusion
// a transposed `useCanUpsert` object would hide in.
function rateCard(title: string): HTMLElement {
  const card = screen.getByRole("heading", { name: title }).closest("section");
  if (!(card instanceof HTMLElement)) {
    throw new Error(`no rate card is headed "${title}"`);
  }
  return card;
}

// Every URL the cards asked for. A withheld card's defining property is that it
// requests NOTHING — a settled denial needs no round trip to be rendered — and
// no assertion about the DOM can show that; only the call log can.
function ratesBackend(allow: GrantSpec, seat: "full" | "read", urls: string[]) {
  return vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input instanceof Request ? input.url : input);
    urls.push(url);
    if (url.endsWith("/v1/me")) {
      return jsonResponse(meFixture({ allow, seat }));
    }
    if (url.includes("/v1/fx-rates")) {
      return jsonResponse({
        data: [
          {
            from_currency: "USD",
            to_currency: "EUR",
            rate: "0.9200000000",
            effective_date: "2026-07-23",
          },
        ],
      });
    }
    if (url.endsWith("/v1/ai/price-sync")) {
      return jsonResponse({ auto_sync: true });
    }
    if (url.endsWith("/v1/ai-model-rates/refresh")) {
      return jsonResponse({
        providers: [
          {
            provider: "anthropic",
            outcome: "updated",
            updated: 2,
            unchanged: 1,
            models: [],
            unlisted: [],
          },
          {
            provider: "openai",
            outcome: "unreachable",
            updated: 0,
            unchanged: 0,
            models: [],
            unlisted: [],
          },
        ],
      });
    }
    if (url.includes("/v1/ai-model-rates")) {
      return jsonResponse({
        data: [
          {
            provider: "anthropic",
            model_id: "claude-opus-4-8",
            input_per_mtok: "5",
            output_per_mtok: "25",
            cache_read_per_mtok: "0.5",
            cache_write_per_mtok: "6.25",
            effective_date: "2026-07-23",
          },
          {
            provider: "openai",
            model_id: "gpt-5.2-mini",
            input_per_mtok: "0.25",
            output_per_mtok: "2",
            cache_read_per_mtok: "0.025",
            cache_write_per_mtok: "0",
            effective_date: "2026-07-01",
          },
          {
            provider: "anthropic",
            model_id: "claude-haiku-4-5",
            input_per_mtok: "1",
            output_per_mtok: "5",
            cache_read_per_mtok: "0.1",
            cache_write_per_mtok: "1.25",
            effective_date: "2026-07-23",
          },
          {
            provider: "fake",
            model_id: "",
            input_per_mtok: "0",
            output_per_mtok: "0",
            cache_read_per_mtok: "0",
            cache_write_per_mtok: "0",
            effective_date: "2026-07-01",
          },
        ],
      });
    }
    return jsonResponse({}, 404);
  });
}

function render(ui: ReactNode) {
  const qc = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={qc}>
      <LocaleProvider>{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

// The two sheets are on different settings pages (FX under Company, model
// prices under AI models). They are rendered as a pair here because one fixture
// backend answers both, and every case below asserts on one of them alone.
function RateSheets() {
  return (
    <>
      <FxRatesCard />
      <ModelPricesCard />
    </>
  );
}

// One mount, and the request log it produced.
function mount(
  allow: GrantSpec,
  seat: "full" | "read" = "full",
): { urls: string[] } {
  const urls: string[] = [];
  vi.stubGlobal("fetch", ratesBackend(allow, seat, urls));
  render(<RateSheets />);
  return { urls };
}

describe("the rate sheets", () => {
  beforeEach(() => {
    globalThis.localStorage?.setItem("margince.workspaceSlug", "acme");
  });

  it("renders both price sheets with their current rows", async () => {
    mount(RATE_SETTER);
    // trimDecimal turns the numeric(20,10) value into a readable 0.92.
    await waitFor(() => expect(screen.getByText("USD")).toBeTruthy());
    expect(screen.getByText("0.92")).toBeTruthy();
    // The day a rate takes effect reads like every other date in Settings,
    // never as the stored ISO string.
    const fx = rateCard("Currency rates");
    expect(
      within(fx).getByText(formatDate("2026-07-23", "en", browserZone())),
    ).toBeTruthy();
    expect(within(fx).queryByText("2026-07-23")).toBeNull();
    expect(screen.getByText("claude-opus-4-8")).toBeTruthy();
    expect(screen.getByText(/\$6\.25$/)).toBeTruthy();

    // The table's name says which rates these are; its headers name columns.
    expect(
      within(rateCard("Currency rates")).getByRole("table", {
        name: "Rates in force",
      }),
    ).toBeTruthy();
  });

  it("groups the model prices under each vendor's name, leaving the test adapter out", async () => {
    mount(RATE_READER);
    await screen.findByText("claude-opus-4-8");
    const card = rateCard("Model prices");
    // One fold per vendor, its name in the summary.
    const anthropic = within(card).getByText("Anthropic").closest("summary");
    const openai = within(card).getByText("OpenAI").closest("summary");
    if (!anthropic || !openai) throw new Error("a vendor has no fold");
    expect(anthropic.compareDocumentPosition(openai)).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
    const anthropicTable = within(card)
      .getByTestId("price-anthropic-claude-opus-4-8-2026-07-23")
      .closest("table");
    expect(
      anthropicTable?.contains(
        within(card).getByTestId("price-anthropic-claude-haiku-4-5-2026-07-23"),
      ),
    ).toBe(true);
    expect(
      anthropicTable?.contains(
        within(card).getByTestId("price-openai-gpt-5.2-mini-2026-07-01"),
      ),
    ).toBe(false);
    expect(within(card).queryByText(/Built-in test provider/)).toBeNull();
    expect(within(card).queryByTestId(/^price-fake/)).toBeNull();
  });

  it("sets the four prices against the end and dates each row in words", async () => {
    mount(RATE_READER);
    const row = await screen.findByTestId(
      "price-anthropic-claude-opus-4-8-2026-07-23",
    );
    for (const price of [/\$5\.00$/, /\$25\.00$/, /\$0\.50$/, /\$6\.25$/]) {
      expect(within(row).getByText(price).className).toContain("datatable-end");
    }
    expect(within(row).getByText("claude-opus-4-8").tagName).toBe("CODE");
    expect(row.textContent).not.toContain("2026-07-23");
  });

  // The dialogs are where a rate is authored, and every box in them is named by
  // a `Field` that owns the input's id. `getByLabelText` is the assertion that
  // the two are wired: a label pointing at nothing cannot be found this way,
  // which is exactly what a hand-rolled `htmlFor` gets wrong silently.
  it("names every box in the currency-rate dialog", async () => {
    const user = userEvent.setup();
    mount(RATE_SETTER);
    await waitFor(() => expect(screen.getByText("USD")).toBeTruthy());

    await user.click(
      within(rateCard("Currency rates")).getByRole("button", {
        name: "Set rate",
      }),
    );
    const dialog = await screen.findByRole("dialog");
    expect(within(dialog).getByRole("textbox", { name: "From" })).toBeTruthy();
    expect(
      within(dialog).getByRole("textbox", { name: "Rate (to base currency)" }),
    ).toBeTruthy();
    expect(
      within(dialog).getByRole("textbox", { name: "Effective" }),
    ).toBeTruthy();
    // An empty dialog says on each required box what it needs, sends nothing,
    // and puts the reader on the first one.
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(await within(dialog).findByText("From is required.")).toBeTruthy();
    expect(
      within(dialog).getByText("Rate (to base currency) is required."),
    ).toBeTruthy();
    await waitFor(() =>
      expect(document.activeElement).toBe(
        within(dialog).getByRole("textbox", { name: "From" }),
      ),
    );
  });

  it("names every box in the model-price dialog", async () => {
    const user = userEvent.setup();
    mount(RATE_SETTER);
    await waitFor(() =>
      expect(screen.getByText("claude-opus-4-8")).toBeTruthy(),
    );

    await user.click(
      within(rateCard("Model prices")).getByRole("button", {
        name: "Add model rate",
      }),
    );
    const dialog = await screen.findByRole("dialog");
    // The provider is a choice from the vendors the product knows, and only
    // once one is chosen is there a model box to offer its models.
    await user.click(
      within(dialog).getByRole("combobox", { name: "Provider" }),
    );
    await user.click(screen.getByRole("option", { name: "gemini" }));
    for (const label of [
      "Provider *",
      "Model *",
      "Input $/M *",
      "Output $/M *",
      "Cache read $/M",
      "Cache write $/M",
      "Effective *",
    ]) {
      expect(within(dialog).getByLabelText(label)).toBeTruthy();
    }
    // Model and the two prices are required; the cache columns default to 0
    // and are left alone by Save.
    await user.click(within(dialog).getByRole("button", { name: "Save" }));
    expect(within(dialog).getByText("Model is required.")).toBeTruthy();
    expect(within(dialog).getByText("Input $/M is required.")).toBeTruthy();
    expect(within(dialog).getByText("Output $/M is required.")).toBeTruthy();
    expect(within(dialog).queryByText(/^Cache .* is required\.$/)).toBeNull();
    expect(document.activeElement).toBe(
      within(dialog).getByRole("combobox", { name: "Model" }),
    );
  });

  it("dates a currency rate on the day picked from the calendar", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-07-20T12:00:00Z"));
    const urls: string[] = [];
    const backend = ratesBackend(RATE_SETTER, "full", urls);
    const posted: unknown[] = [];
    vi.stubGlobal("fetch", async (input: Request) => {
      if (input.method === "POST") posted.push(await input.clone().json());
      return backend(input);
    });
    const user = userEvent.setup();
    render(<RateSheets />);
    await waitFor(() => expect(screen.getByText("USD")).toBeTruthy());

    await user.click(
      within(rateCard("Currency rates")).getByRole("button", {
        name: "Set rate",
      }),
    );
    const dialog = within(await screen.findByRole("dialog"));
    await user.type(dialog.getByRole("textbox", { name: "From" }), "GBP");
    await user.type(
      dialog.getByRole("textbox", { name: "Rate (to base currency)" }),
      "1.17",
    );
    await user.click(dialog.getByRole("button", { name: /^Choose date/ }));
    await user.click(
      await screen.findByRole("button", { name: "Friday, 24 July 2026" }),
    );
    await user.click(dialog.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(posted).toEqual([
        { from_currency: "GBP", rate: "1.17", effective_date: "2026-07-24" },
      ]),
    );
  });

  // A rate is read by the shared parser and sent as decimal text. A comma is
  // the decimal sign, and the digits are kept as written.
  it("sends a currency rate typed with a comma as a plain decimal", async () => {
    const backend = ratesBackend(RATE_SETTER, "full", []);
    const posted: unknown[] = [];
    vi.stubGlobal("fetch", async (input: Request) => {
      if (input.method === "POST") posted.push(await input.clone().json());
      return backend(input);
    });
    const user = userEvent.setup();
    render(<RateSheets />);
    await waitFor(() => expect(screen.getByText("USD")).toBeTruthy());
    await user.click(
      within(rateCard("Currency rates")).getByRole("button", {
        name: "Set rate",
      }),
    );
    const dialog = within(await screen.findByRole("dialog"));
    await user.type(dialog.getByRole("textbox", { name: "From" }), "GBP");
    const rate = dialog.getByRole("textbox", {
      name: "Rate (to base currency)",
    });
    await user.type(rate, "1.170,5");
    expect(
      dialog.getByText(
        "Enter the number without thousands separators, such as 1234.5.",
      ),
    ).toBeTruthy();
    await user.clear(rate);
    await user.type(rate, "0,00000012");
    await user.click(dialog.getByRole("button", { name: "Save" }));
    await waitFor(() =>
      expect(posted).toMatchObject([
        { from_currency: "GBP", rate: "0.00000012" },
      ]),
    );
  });

  // The checks the server makes that need no database run before sending, in
  // the words the server's refusal gets.
  it("refuses a code that is no currency, the base currency and a rate of zero before sending", async () => {
    const backend = ratesBackend(RATE_SETTER, "full", []);
    const posted: unknown[] = [];
    vi.stubGlobal("fetch", async (input: Request) => {
      if (input.method === "POST") posted.push(await input.clone().json());
      if (input.url.endsWith("/v1/installation/settings")) {
        return jsonResponse({ base_currency: "EUR" });
      }
      return backend(input);
    });
    const user = userEvent.setup();
    render(<RateSheets />);
    await waitFor(() => expect(screen.getByText("USD")).toBeTruthy());
    await user.click(
      within(rateCard("Currency rates")).getByRole("button", {
        name: "Set rate",
      }),
    );
    const dialog = within(await screen.findByRole("dialog"));
    const from = dialog.getByRole("textbox", { name: "From" });
    await user.type(from, "ZZZ");
    await user.type(
      dialog.getByRole("textbox", { name: "Rate (to base currency)" }),
      "0",
    );
    await user.click(dialog.getByRole("button", { name: "Save" }));
    expect(
      await dialog.findByText(en["settings.rates.fromNotCurrency"]),
    ).toBeTruthy();
    expect(dialog.getByText(en["settings.rates.rateNotPositive"])).toBeTruthy();
    expect(from.getAttribute("aria-invalid")).toBe("true");

    await user.clear(from);
    expect(dialog.queryByText(en["settings.rates.fromNotCurrency"])).toBeNull();
    await user.type(from, "eur");
    await user.click(dialog.getByRole("button", { name: "Save" }));
    expect(
      await dialog.findByText(en["settings.rates.fromIsBase"]),
    ).toBeTruthy();
    expect(posted).toEqual([]);
  });

  it("puts the server's refusal of a past date on the Effective field", async () => {
    const backend = ratesBackend(RATE_SETTER, "full", []);
    vi.stubGlobal("fetch", async (input: Request) => {
      if (input.method === "POST") {
        return jsonResponse(
          {
            status: 422,
            code: "validation_error",
            detail: "effective_date cannot be in the past",
            details: {
              errors: [
                {
                  field: "effective_date",
                  code: "fx_rate_past",
                  message: "effective_date cannot be in the past",
                },
              ],
            },
          },
          422,
        );
      }
      return backend(input);
    });
    const user = userEvent.setup();
    render(<RateSheets />);
    await waitFor(() => expect(screen.getByText("USD")).toBeTruthy());
    await user.click(
      within(rateCard("Currency rates")).getByRole("button", {
        name: "Set rate",
      }),
    );
    const dialog = within(await screen.findByRole("dialog"));
    await user.type(dialog.getByRole("textbox", { name: "From" }), "GBP");
    await user.type(
      dialog.getByRole("textbox", { name: "Rate (to base currency)" }),
      "1.17",
    );
    await user.click(dialog.getByRole("button", { name: "Save" }));
    expect(await dialog.findByText(en["settings.rates.pastDate"])).toBeTruthy();
    const effective = dialog.getByRole("textbox", { name: "Effective" });
    await waitFor(() => expect(document.activeElement).toBe(effective));
    expect(dialog.queryByText(/cannot be in the past/)).toBeNull();
    expect(dialog.queryByText(en["settings.rates.notSaved"])).toBeNull();
  });

  it("refuses a model price effective before the reader's today", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-07-20T12:00:00Z"));
    const user = userEvent.setup();
    mount(RATE_SETTER);
    await screen.findByText("claude-opus-4-8");

    await user.click(
      within(rateCard("Model prices")).getByRole("button", {
        name: "Add model rate",
      }),
    );
    const dialog = within(await screen.findByRole("dialog"));
    await user.click(dialog.getByRole("button", { name: /^Choose date/ }));
    const lastWeek = await screen.findByRole("button", {
      name: /^Wednesday, 15 July 2026, Earliest date: /,
    });
    expect(lastWeek.getAttribute("aria-disabled")).toBe("true");
  });

  it("offers the floor as Today, so the marked day is one the price may take", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-07-20T12:00:00Z"));
    const user = userEvent.setup();
    mount(RATE_SETTER);
    await screen.findByText("claude-opus-4-8");
    await user.click(
      within(rateCard("Model prices")).getByRole("button", {
        name: "Add model rate",
      }),
    );
    const dialog = within(await screen.findByRole("dialog"));
    const effective = dialog.getByRole<HTMLInputElement>("textbox", {
      name: "Effective",
    });
    await user.tripleClick(effective);
    await user.keyboard("{Backspace}");
    await user.click(dialog.getByRole("button", { name: /^Choose date/ }));
    const pickToday = await screen.findByRole("button", { name: "Today" });
    expect(pickToday).toHaveProperty("disabled", false);
    await user.click(pickToday);

    expect(effective.value).toBe(
      formatDate(todayIn(AI_RATE_DAY_ZONE), "en", browserZone()),
    );
  });

  // West of UTC in the evening the server's UTC day is already tomorrow, and it
  // refuses an earlier day as rate_past. The form opens on the server's day.
  it("opens a model price on the day the server counts as today", async () => {
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-07-21T02:00:00Z"));
    viewer.zone = "America/New_York";
    const user = userEvent.setup();
    mount(RATE_SETTER);
    await screen.findByText("claude-opus-4-8");
    await user.click(
      within(rateCard("Model prices")).getByRole("button", {
        name: "Add model rate",
      }),
    );
    const dialog = within(await screen.findByRole("dialog"));
    const effective = dialog.getByRole<HTMLInputElement>("textbox", {
      name: "Effective",
    });

    expect(effective.value).toBe(formatDate("2026-07-21", "en", browserZone()));
  });

  it("shows write affordances for an admin", async () => {
    mount(PRICE_ADMIN);
    await waitFor(() => expect(screen.getByText("USD")).toBeTruthy());
    expect(screen.getByRole("button", { name: "Set rate" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Add model rate" })).toBeTruthy();
    // Each card exposes its own refresh to an admin: the currency sheet asks
    // for proposals, the model sheet re-prices from the catalogue.
    expect(
      screen.getAllByRole("button", { name: "Refresh from sources" }),
    ).toHaveLength(1);
    expect(
      screen.getAllByRole("button", { name: "Refresh model prices" }),
    ).toHaveLength(1);
    // And nothing about reading is withheld or read-only for them.
    expect(screen.queryByText(/your role cannot change rates/i)).toBeNull();
  });

  // The button runs the refresh inline, so its answer lands on the sheet it
  // wrote: how many prices moved, and which vendors could not be asked.
  it("reports what a refresh wrote and which vendors it could not reach", async () => {
    const user = userEvent.setup();
    mount(PRICE_ADMIN);
    await user.click(
      await screen.findByRole("button", { name: "Refresh model prices" }),
    );
    const report = await screen.findByRole("status");
    expect(report.textContent).toContain("2 prices written");
    expect(report.textContent).toContain("Unreachable: openai");
  });

  it("hides write affordances for a role granted the read alone", async () => {
    mount(RATE_READER);
    await waitFor(() => expect(screen.getByText("USD")).toBeTruthy());
    expect(screen.queryByRole("button", { name: "Set rate" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Add model rate" })).toBeNull();
  });

  // One object at a time. A fixture that grants both sheets at once cannot tell
  // a correct binding from a transposed one — each card would find its grant
  // either way — so each case below grants exactly one and requires the OTHER
  // card to stay read-only.
  it("opens the FX sheet alone on an fx_rate create grant", async () => {
    mount({ fx_rate: ["read", "create"], ai_model_rate: ["read"] });
    await waitFor(() => expect(screen.getByText("USD")).toBeTruthy());

    const fx = rateCard("Currency rates");
    expect(within(fx).getByRole("button", { name: "Set rate" })).toBeTruthy();
    expect(
      within(fx).getByRole("button", { name: "Refresh from sources" }),
    ).toBeTruthy();

    // The model sheet still renders its rows — it is the WRITING that is
    // withheld, not the reading.
    const model = rateCard("Model prices");
    expect(within(model).getByText("claude-opus-4-8")).toBeTruthy();
    expect(
      within(model).queryByRole("button", { name: "Add model rate" }),
    ).toBeNull();
    expect(
      within(model).queryByRole("button", { name: "Refresh model prices" }),
    ).toBeNull();
  });

  it("opens the model sheet alone on an ai_model_rate update grant", async () => {
    // `update` and not `create`: the upsert admits either, so the mirror case
    // also proves the card asks the union rather than one hard-coded verb.
    mount({ fx_rate: ["read"], ai_model_rate: ["read", "update"] });
    await waitFor(() => expect(screen.getByText("USD")).toBeTruthy());

    const model = rateCard("Model prices");
    expect(
      within(model).getByRole("button", { name: "Add model rate" }),
    ).toBeTruthy();
    // A refresh also adds rows, which an update grant alone would be refused.
    expect(
      within(model).queryByRole("button", { name: "Refresh model prices" }),
    ).toBeNull();

    const fx = rateCard("Currency rates");
    expect(within(fx).getByText("USD")).toBeTruthy();
    expect(within(fx).queryByRole("button", { name: "Set rate" })).toBeNull();
    expect(
      within(fx).queryByRole("button", { name: "Refresh from sources" }),
    ).toBeNull();
  });

  it("offers no write affordance on either sheet for a read-only grant on both", async () => {
    // A read grant is not an absent one: the object IS in the snapshot, with
    // every write verb false. A card that checked only for the object's
    // presence would open here.
    mount(RATE_READER);
    await waitFor(() => expect(screen.getByText("USD")).toBeTruthy());
    expect(screen.getByText("claude-opus-4-8")).toBeTruthy();

    expect(screen.queryByRole("button", { name: "Set rate" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Add model rate" })).toBeNull();
    expect(
      screen.queryAllByRole("button", { name: "Refresh from sources" }),
    ).toEqual([]);
    expect(
      screen.queryAllByRole("button", { name: "Refresh model prices" }),
    ).toEqual([]);

    // The read-only posture is stated once per readable sheet, and the withheld
    // reason is not. Stacking both would explain one denial twice, two ways.
    // The model prices say it on their switch.
    expect(
      within(rateCard("Currency rates")).getByText(
        /your role cannot change rates/i,
      ),
    ).toBeTruthy();
    expect(
      within(rateCard("Model prices")).getByText(
        "Your role cannot change this.",
      ),
    ).toBeTruthy();
    expect(
      screen.queryByText(/only an administrator or operations user can see/i),
    ).toBeNull();
  });

  // Withheld, not absent. Currency rates sits on the Company page a
  // read_only seat now opens for its other cards, so a sheet that vanished
  // would read as "this installation converts nothing" — and a sheet that
  // fetched its list in order to render the 403 would read as a broken page
  // with a Retry that can only be refused again.
  it("withholds the FX sheet without fx_rate:read and requests no rates", async () => {
    const { urls } = mount({ ai_model_rate: ["read"] });

    // Awaited on the sibling's rows, which land after /me and therefore after
    // the withheld notice: an assertion that stopped at the notice would judge
    // the FX request log while the model list was still in flight.
    expect(await screen.findByText("claude-opus-4-8")).toBeTruthy();
    expect(
      screen.getByText(
        /only an administrator or operations user can see currency rates/i,
      ),
    ).toBeTruthy();
    const fx = rateCard("Currency rates");
    expect(within(fx).queryByText("USD")).toBeNull();
    // Not the table with an empty body either: the withheld card draws no
    // sheet at all.
    expect(within(fx).queryByRole("table")).toBeNull();
    // One explanation, not two: the read-only caption belongs to a sheet the
    // reader may actually read.
    expect(within(fx).queryByText(/your role cannot change rates/i)).toBeNull();
    expect(urls.some((url) => url.includes("/v1/fx-rates"))).toBe(false);

    // The model sheet is unaffected — the grants are per object, and a card that
    // read the wrong one would fail here rather than pass by symmetry.
    const model = rateCard("Model prices");
    expect(within(model).getByText("claude-opus-4-8")).toBeTruthy();
    expect(urls.some((url) => url.includes("/v1/ai-model-rates"))).toBe(true);
  });

  it("withholds the model sheet without ai_model_rate:read and requests no rates", async () => {
    const { urls } = mount({ fx_rate: ["read"] });

    // Awaited on the readable sibling, for the same reason as above.
    expect(await screen.findByText("USD")).toBeTruthy();
    expect(screen.getByText("Model prices are not yours to see.")).toBeTruthy();
    const model = rateCard("Model prices");
    expect(within(model).queryByText("claude-opus-4-8")).toBeNull();
    expect(
      within(model).queryByText(/your role cannot change rates/i),
    ).toBeNull();
    expect(urls.some((url) => url.includes("/v1/ai-model-rates"))).toBe(false);

    const fx = rateCard("Currency rates");
    expect(within(fx).getByText("USD")).toBeTruthy();
    expect(urls.some((url) => url.includes("/v1/fx-rates"))).toBe(true);
  });

  // The read SEAT is a write ceiling, not a read one (A62/ADR-0047): the server
  // clamps it on the HTTP method, so these grants still read both sheets. The
  // card must therefore reach for the read grant alone when it decides whether
  // to withhold — telling an admin looking at the table that only an admin may
  // see it would be a lie about which permission ran out.
  it("keeps both sheets readable for a rate-setter on a read seat", async () => {
    mount(RATE_SETTER, "read");
    await waitFor(() => expect(screen.getByText("USD")).toBeTruthy());
    expect(screen.getByText("claude-opus-4-8")).toBeTruthy();

    expect(
      screen.queryByText(/only an administrator or operations user can see/i),
    ).toBeNull();
    expect(
      within(rateCard("Currency rates")).getByText(
        /your role cannot change rates/i,
      ),
    ).toBeTruthy();
    const model = rateCard("Model prices");
    expect(
      within(model).getByText("Your role cannot change this."),
    ).toBeTruthy();
    expect(
      within(model).getByRole("switch", { name: "Auto-sync daily" }),
    ).toHaveProperty("disabled", true);
    expect(screen.queryByRole("button", { name: "Set rate" })).toBeNull();
    expect(screen.queryByRole("button", { name: "Add model rate" })).toBeNull();
  });
});
