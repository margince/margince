/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { pickOption } from "../../design-system/select-testing";
import { meFixture } from "../../testing/mefixture";
import { DealsScreen } from "./deals";
import {
  deal,
  jsonResponse,
  REP_GRANTS,
  render,
  stages,
  stubBackend,
} from "./deals.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
  localStorage.clear();
});

describe("DealsScreen filters", () => {
  it("switching pipeline scopes the deals fetch to that pipeline_id", async () => {
    const user = userEvent.setup();
    const urls: string[] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], {
        onDealsUrl: (u) => urls.push(u),
        pipelines: [
          {
            id: "pl",
            name: "Sales",
            is_default: true,
            position: 0,
            stages,
          },
          {
            id: "pl2",
            name: "Renewals",
            is_default: false,
            position: 1,
            stages,
          },
        ],
      }),
    );
    render(<DealsScreen />);
    await screen.findByText("Fleet retrofit");
    await pickOption(user, screen.getByLabelText("Pipeline"), "Renewals");
    await waitFor(() =>
      expect(urls.some((u) => u.includes("pipeline_id=pl2"))).toBe(true),
    );
  });

  // The board always shows one pipeline, so an unset choice would fall straight
  // back to the default one. The pipeline list therefore offers pipelines
  // only. The stage filter's "all" entry clears a query filter, which the board
  // can show, so that one stays.
  it("offers pipelines only, while the stage filter keeps its all-stages entry", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], {
        pipelines: [
          {
            id: "pl",
            name: "Sales",
            is_default: true,
            position: 0,
            stages,
          },
          {
            id: "pl2",
            name: "Renewals",
            is_default: false,
            position: 1,
            stages,
          },
        ],
      }),
    );
    render(<DealsScreen />);
    await screen.findByText("Fleet retrofit");

    await user.click(screen.getByLabelText("Pipeline"));
    expect(
      within(screen.getByRole("listbox"))
        .getAllByRole("option")
        .map((option) => option.textContent),
    ).toEqual(["Sales", "Renewals"]);
    await user.keyboard("{Escape}");

    // Stage is a filter chip rather than a picker of its own, so its
    // all-stages entry sits one step inside the Filter menu. It must still be
    // there, because that entry is how a chosen stage is cleared.
    // "Stage" also names a column header, so the attribute is picked from
    // inside the menu that is open rather than by a plain name match.
    await user.click(screen.getByRole("button", { name: "Table" }));
    await user.click(screen.getByRole("button", { name: "Filter" }));
    const menu = screen.getByRole("group", { name: "Filter" });
    await user.click(within(menu).getByRole("button", { name: "Stage" }));
    expect(
      within(menu).getByRole("radio", { name: "All stages" }),
    ).toBeTruthy();
  });

  it("the stalled filter adds stalled=true to the deals query", async () => {
    const urls: string[] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], { onDealsUrl: (u) => urls.push(u) }),
    );
    render(<DealsScreen />);
    await screen.findByText("Fleet retrofit");
    // The stalled filter lives on the table view, not the board.
    await userEvent.click(screen.getByRole("button", { name: "Table" }));

    // The Filter button's attribute step and its value step both carry the
    // "Stalled only" label, because the chip's option shares the attribute's
    // name. So each step is picked from inside the menu that is open at that
    // moment, rather than by a plain (ambiguous) name match.
    await userEvent.click(screen.getByRole("button", { name: "Filter" }));
    const menu = screen.getByRole("group", { name: "Filter" });
    await userEvent.click(
      within(menu).getByRole("button", { name: "Stalled only" }),
    );
    await userEvent.click(
      within(menu).getByRole("radio", { name: "Stalled only" }),
    );

    await waitFor(() =>
      expect(urls.some((u) => u.includes("stalled=true"))).toBe(true),
    );
  });

  // The forecast's buckets, as a dial. Five tiles on the analytics page
  // partition the pipeline into named buckets, and each needs a door to its
  // deals. "Which deals are in commit" is the first question asked of one.
  it("the forecast filter narrows the deals query to one bucket", async () => {
    const urls: string[] = [];
    vi.stubGlobal(
      "fetch",
      stubBackend([deal({})], { onDealsUrl: (u) => urls.push(u) }),
    );
    render(<DealsScreen />);
    await screen.findByText("Fleet retrofit");
    await userEvent.click(screen.getByRole("button", { name: "Table" }));

    await userEvent.click(screen.getByRole("button", { name: "Filter" }));
    const menu = screen.getByRole("group", { name: "Filter" });
    await userEvent.click(
      within(menu).getByRole("button", { name: "Forecast" }),
    );
    await userEvent.click(within(menu).getByRole("radio", { name: "Commit" }));

    await waitFor(() =>
      expect(urls.some((u) => u.includes("forecast_category=commit"))).toBe(
        true,
      ),
    );
  });
});

// Partner choices are limited to companies this reader may open.

describe("the partner filter", () => {
  it("narrows the list to one named partner", async () => {
    const urls: string[] = [];
    const d = deal({ id: "d1", name: "Fleet retrofit" });
    vi.stubGlobal("fetch", (request: Request) => {
      urls.push(request.url);
      if (request.url.includes("/partners")) {
        return Promise.resolve(
          jsonResponse({
            data: [{ company_id: "o1", cert_status: "certified" }],
            page: { next_cursor: null },
          }),
        );
      }
      return Promise.resolve(stubBackend([d], { single: d })(request));
    });

    render(<DealsScreen />);
    await screen.findByText("Fleet retrofit");
    await userEvent.click(screen.getByRole("button", { name: "Table" }));

    await userEvent.click(screen.getByRole("button", { name: "Filter" }));
    const menu = screen.getByRole("group", { name: "Filter" });
    await userEvent.click(
      within(menu).getByRole("button", { name: "Partner" }),
    );
    // The option is the company's name, resolved from the company list. It is
    // never the bare id, which names nothing to a reader.
    await userEvent.click(within(menu).getByRole("radio", { name: "Acme" }));

    await waitFor(() =>
      expect(urls.some((u) => u.includes("partner_company_id=o1"))).toBe(true),
    );
  });

  it("sends the partner to the board totals too, not only the deal list", async () => {
    // The board's per-column totals come from the deals-by-stage report with
    // the screen's own dials. A dial the screen offers and the report refuses
    // answers 422. The board then counts the cards it happens to hold, which
    // looks just like a working total.
    const bodies: unknown[] = [];
    const d = deal({ id: "d1", name: "Fleet retrofit" });
    vi.stubGlobal("fetch", async (request: Request) => {
      if (request.url.includes("/partners")) {
        return jsonResponse({
          data: [{ company_id: "o1", cert_status: "certified" }],
          page: { next_cursor: null },
        });
      }
      if (request.url.includes("/reports/deals-by-stage")) {
        bodies.push(await request.clone().json());
      }
      return stubBackend([d], { single: d })(request);
    });

    render(<DealsScreen />);
    await screen.findByText("Fleet retrofit");
    await userEvent.click(screen.getByRole("button", { name: "Table" }));
    // The partner dial itself is the subject, so it is pressed, not addressed.
    const menu = screen.getByRole("group", { name: "Filter" });
    await userEvent.click(
      within(menu).getByRole("button", { name: "Partner" }),
    );
    await userEvent.click(within(menu).getByRole("radio", { name: "Acme" }));

    await waitFor(() =>
      expect(
        bodies.some(
          (b) =>
            (b as { filters?: Record<string, unknown> }).filters
              ?.partner_company_id === "o1",
        ),
      ).toBe(true),
    );
  });

  it("offers no partner filter when the installation has no partners", async () => {
    const d = deal({ id: "d1", name: "Fleet retrofit" });
    vi.stubGlobal("fetch", (request: Request) => {
      if (request.url.includes("/partners")) {
        return Promise.resolve(
          jsonResponse({ data: [], page: { next_cursor: null } }),
        );
      }
      return Promise.resolve(stubBackend([d], { single: d })(request));
    });

    render(<DealsScreen />);
    await screen.findByText("Fleet retrofit");
    await userEvent.click(screen.getByRole("button", { name: "Table" }));
    await userEvent.click(screen.getByRole("button", { name: "Filter" }));

    // A picker with nothing in it asks a question that has no answers.
    const menu = screen.getByRole("group", { name: "Filter" });
    expect(within(menu).queryByRole("button", { name: "Partner" })).toBeNull();
  });
});

// No dial narrows the list without a chip that names it. A dial the reader
// cannot use stays in the address and is not sent.
describe("DealsScreen range dials the reader cannot use", () => {
  function backend({
    baseCurrency,
    readsCompanies,
    urls,
  }: Readonly<{
    baseCurrency?: string;
    readsCompanies: boolean;
    urls: string[];
  }>) {
    const stub = stubBackend([deal({})], {
      onDealsUrl: (url) => urls.push(url),
    });
    const allow = readsCompanies
      ? { ...REP_GRANTS, company: ["read"] as const }
      : REP_GRANTS;
    return async (request: Request) => {
      if (request.url.includes("/installation/settings")) {
        return jsonResponse({ base_currency: baseCurrency });
      }
      if (new URL(request.url).pathname.endsWith("/me")) {
        return jsonResponse({
          user: { id: "u-me", display_name: "Me", status: "active" },
          roles: ["rep"],
          teams: [],
          authorization: meFixture({ allow }).authorization,
        });
      }
      return stub(request);
    };
  }

  const ADDRESS = "#/deals?value=..5000000&country=DE";

  it("sends the value and the country once both can be read", async () => {
    const urls: string[] = [];
    window.location.hash = ADDRESS;
    vi.stubGlobal(
      "fetch",
      backend({ baseCurrency: "EUR", readsCompanies: true, urls }),
    );
    render(<DealsScreen />);
    await waitFor(() =>
      expect(
        urls.some(
          (url) =>
            url.includes("value_max=5000000") && url.includes("country=DE"),
        ),
      ).toBe(true),
    );
  });

  it("holds back a value bound while the base currency is unknown", async () => {
    const urls: string[] = [];
    window.location.hash = ADDRESS;
    vi.stubGlobal("fetch", backend({ readsCompanies: true, urls }));
    render(<DealsScreen />);
    await screen.findByText("Fleet retrofit");
    await waitFor(() =>
      expect(urls.some((url) => url.includes("country=DE"))).toBe(true),
    );
    expect(urls.some((url) => url.includes("value_max"))).toBe(false);
  });

  it("holds back a country from a reader who cannot read companies", async () => {
    const urls: string[] = [];
    window.location.hash = ADDRESS;
    vi.stubGlobal(
      "fetch",
      backend({ baseCurrency: "EUR", readsCompanies: false, urls }),
    );
    render(<DealsScreen />);
    await waitFor(() =>
      expect(urls.some((url) => url.includes("value_max=5000000"))).toBe(true),
    );
    expect(urls.some((url) => url.includes("country="))).toBe(false);
  });
});
