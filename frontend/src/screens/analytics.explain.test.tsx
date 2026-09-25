/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../i18n/en";
import { AnalyticsScreen } from "./analytics";
import {
  derivationCellCurrency,
  derivationColumns,
  parseDerivationQuery,
} from "./analytics.explain";
import { render, reportsStub } from "./analytics.testkit";

// "Explain this number" in both of its hosts: the panel under a report card,
// which resolves the RESULT's handle, and the drawer beside a row, which
// resolves that ROW's. The two share one body, so what each test proves about
// the body holds for both; what is host-specific is which handle is read.

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// A row's own handle binds more than the result's does: the report-level link
// the testkit mints carries no pipeline, so a request that names one came from
// the row.
const ROW_HANDLE =
  "/v1/reports/pipeline-current/derivation?by=stage_id&agg=count::deal_count&stage_id=pl-s1&pipeline_id=pl";

const stageRow = (extra: Record<string, unknown> = {}) => ({
  stage_id: "pl-s1",
  raw_minor: 100000,
  deal_count: 2,
  ...extra,
});

const derivation = (extra: Record<string, unknown> = {}) => ({
  report: "pipeline-current",
  definition: "Sum over open deals",
  plan: {},
  columns: ["name"],
  rows: [{ name: "Fleet retrofit" }],
  ...extra,
});

type User = ReturnType<typeof userEvent.setup>;

// The report cards live behind the Deals tab; Forecast is where a reader lands.
async function openPipeline(user: User) {
  await user.click(await screen.findByRole("button", { name: "Deals" }));
}

// Every card on the Deals tab carries its own panel toggle, so the first one
// is the stage table's: the card the rows below belong to.
async function openPanel(user: User) {
  await openPipeline(user);
  const toggles = await screen.findAllByRole("button", {
    name: en["explain.open"],
  });
  await user.click(toggles[0]);
}

async function openRowDrawer(user: User) {
  await openPipeline(user);
  await user.click(
    await screen.findByRole("button", { name: "Explain Qualify" }),
  );
  return screen.findByRole("dialog");
}

describe("the result's explain panel", () => {
  it("fetches the derivation and renders source rows, not raw JSON", async () => {
    const user = userEvent.setup();
    const derivationUrls: string[] = [];
    vi.stubGlobal(
      "fetch",
      reportsStub({
        onDerivation: (u) => derivationUrls.push(u),
        derivation: derivation(),
      }),
    );
    render(<AnalyticsScreen />);
    await openPanel(user);
    expect(await screen.findByText("Fleet retrofit")).toBeTruthy();
    expect(screen.queryByText(/"plan":/)).toBeNull();
    // The equality predicate from derivation_url must survive to the request —
    // by/agg alone would explain the wrong slice.
    expect(derivationUrls[0]).toContain("stage_id=pl-s1");
    expect(derivationUrls[0]).toContain("by=stage_id");
  });

  // A link minted before the handle carried an instant. The figures were
  // recomputed at a NEW moment, so a rate sheet effective in between makes them
  // disagree with the number they explain.
  it("says the figures were recalculated when the link pinned no instant", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      reportsStub({ derivation: derivation({ as_of_pinned: false }) }),
    );
    render(<AnalyticsScreen />);
    await openPanel(user);
    expect(await screen.findByText(en["explain.mayHaveMoved"])).toBeTruthy();
    // Saying they were recomputed is the fix; withholding them is not.
    expect(screen.getByText("Fleet retrofit")).toBeTruthy();
  });

  // A caveat on every drill-through is a caveat nobody reads.
  it("stays silent when the link pinned the headline's instant", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      reportsStub({ derivation: derivation({ as_of_pinned: true }) }),
    );
    render(<AnalyticsScreen />);
    await openPanel(user);
    expect(await screen.findByText("Fleet retrofit")).toBeTruthy();
    expect(screen.queryByText(en["explain.mayHaveMoved"])).toBeNull();
  });
});

describe("a row's explain drawer", () => {
  it("requests the row's own handle, not the result's", async () => {
    const user = userEvent.setup();
    const derivationUrls: string[] = [];
    vi.stubGlobal(
      "fetch",
      reportsStub({
        stageRows: [stageRow({ derivation_url: ROW_HANDLE })],
        onDerivation: (u) => derivationUrls.push(u),
        derivation: derivation(),
      }),
    );
    render(<AnalyticsScreen />);
    const drawer = await openRowDrawer(user);
    expect(await within(drawer).findByText("Fleet retrofit")).toBeTruthy();
    expect(derivationUrls).toHaveLength(1);
    expect(derivationUrls[0]).toContain("pipeline_id=pl");
    expect(derivationUrls[0]).toContain("stage_id=pl-s1");
    // The drawer names the figure it explains, so the reader who pressed
    // "Explain Qualify" can see that Qualify is what opened.
    expect(within(drawer).getByText("Qualify")).toBeTruthy();
  });

  it("draws no trigger on a row the server sent without a handle", async () => {
    vi.stubGlobal("fetch", reportsStub({ stageRows: [stageRow()] }));
    render(<AnalyticsScreen />);
    await openPipeline(userEvent.setup());
    // The row itself renders, door and all: only the trigger is absent.
    expect(await screen.findByRole("link", { name: /2/ })).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: "Explain Qualify" }),
    ).toBeNull();
  });

  it("puts a trigger on a forecast tile whose row carries a handle", async () => {
    vi.stubGlobal(
      "fetch",
      reportsStub({
        forecastRows: [
          {
            forecast_category: "commit",
            raw_minor: 100000,
            deal_count: 1,
            derivation_url: ROW_HANDLE,
          },
        ],
      }),
    );
    render(<AnalyticsScreen />);
    await openPipeline(userEvent.setup());
    expect(
      await screen.findByRole("button", { name: "Explain Commit" }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", { name: "Explain Best case" }),
    ).toBeNull();
  });

  it("closes on Escape and hands focus back to its trigger", async () => {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      reportsStub({
        stageRows: [stageRow({ derivation_url: ROW_HANDLE })],
        derivation: derivation(),
      }),
    );
    render(<AnalyticsScreen />);
    const drawer = await openRowDrawer(user);
    await within(drawer).findByText("Fleet retrofit");
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
    expect(document.activeElement).toBe(
      screen.getByRole("button", { name: "Explain Qualify" }),
    );
  });
});

// The body both hosts share: what a mask took out, and what a cap left off.
describe("what the explanation owns up to", () => {
  async function drawerWith(extra: Record<string, unknown>) {
    const user = userEvent.setup();
    vi.stubGlobal(
      "fetch",
      reportsStub({
        stageRows: [stageRow({ derivation_url: ROW_HANDLE })],
        derivation: derivation(extra),
      }),
    );
    render(<AnalyticsScreen />);
    const drawer = await openRowDrawer(user);
    await within(drawer).findByText("Fleet retrofit");
    return drawer;
  }

  it("says how many records a field mask left out of the figure", async () => {
    const drawer = await drawerWith({ excluded_by_permission: 2 });
    expect(
      within(drawer).getByText(
        en["explain.excluded_other"].replace("{count}", "2"),
      ),
    ).toBeTruthy();
  });

  // Zero means a mask applied and took nothing: there is nothing to own up to.
  it("says nothing when the mask excluded no record", async () => {
    const drawer = await drawerWith({ excluded_by_permission: 0 });
    expect(within(drawer).queryByText(/left out of this number/)).toBeNull();
  });

  // The server caps the rows and still counts them all; a capped list that
  // did not say so would read as the whole set.
  it("states the rows the cap left off", async () => {
    const drawer = await drawerWith({ total_rows: 1201 });
    expect(
      within(drawer).getByText(
        en["state.partialCount"].replace("{count}", "1,200"),
      ),
    ).toBeTruthy();
  });
});

describe("parseDerivationQuery", () => {
  it("pulls by/agg + predicate params from a derivation_url", () => {
    const q = parseDerivationQuery(
      "/v1/reports/deals-by-stage/derivation?by=stage_id&agg=sum:amount_minor:raw&stage_id=s1",
    );
    expect(q.by).toEqual(["stage_id"]);
    expect(q.agg).toEqual(["sum:amount_minor:raw"]);
    expect(q.stage_id).toBe("s1");
  });
});

// A drill-through row carries money in two different currencies at once, and
// which one a cell is written in depends on the COLUMN.
//
// `pipeline-current` converts server-side and exposes `amount_base_minor`, in
// the installation's base currency. The forecast does not convert: it exposes
// the deal's own `amount_minor` with the currency it was written in on the
// same row. Formatting both against the base currency puts a euro sign on a
// dollar deal — a wrong number wearing a right-looking symbol.
describe("drill-through money", () => {
  it("writes a converted measure in the base currency", () => {
    const row = { amount_base_minor: 500000, currency: "USD" };
    expect(derivationCellCurrency("amount_base_minor", row, "EUR")).toBe("EUR");
  });

  it("writes an unconverted measure in the deal's own currency", () => {
    const row = { amount_minor: 500000, currency: "USD" };
    expect(derivationCellCurrency("amount_minor", row, "EUR")).toBe("USD");
  });

  // A row that names no currency has nothing to write the figure in. Falling
  // back to the base currency would be a guess presented as a fact.
  it("names no currency for an unconverted measure on a row without one", () => {
    expect(derivationCellCurrency("amount_minor", {}, "EUR")).toBeNull();
    expect(
      derivationCellCurrency("amount_minor", { currency: "" }, "EUR"),
    ).toBeNull();
  });
});

// The id is noise beside a name — but only when every row HAS one. Labelling
// is per row, so a reader who may not read one record gets a label column
// with a gap in it.
describe("drill-through columns", () => {
  const shaped = (columns: string[], rows: Record<string, unknown>[]) =>
    ({ columns, rows }) as unknown as Parameters<typeof derivationColumns>[0];

  it("drops the id once every row is named", () => {
    expect(
      derivationColumns(
        shaped(
          ["id", "label", "amount_minor"],
          [
            { id: "a", label: "Acme" },
            { id: "b", label: "Globex" },
          ],
        ),
      ),
    ).toEqual(["label", "amount_minor"]);
  });

  // The row whose name was withheld is the one a reader can least account
  // for. Dropping the id here would leave it showing a blank and nothing else.
  it("keeps the id when any row's name was withheld", () => {
    expect(
      derivationColumns(
        shaped(
          ["id", "label", "amount_minor"],
          [{ id: "a", label: "Acme" }, { id: "b" }],
        ),
      ),
    ).toEqual(["label", "id", "amount_minor"]);
  });

  // A plan may select the name after every measure; the reader still needs to
  // know which record a row is before what it is worth.
  it("leads with the record's name whatever order the plan selected", () => {
    expect(
      derivationColumns(
        shaped(
          ["currency", "owner_id", "amount_base_minor", "label"],
          [{ label: "Acme" }],
        ),
      ),
    ).toEqual(["label", "currency", "owner_id", "amount_base_minor"]);
  });

  it("keeps the id when no row could be named", () => {
    expect(
      derivationColumns(shaped(["id", "amount_minor"], [{ id: "a" }])),
    ).toEqual(["id", "amount_minor"]);
  });
});
