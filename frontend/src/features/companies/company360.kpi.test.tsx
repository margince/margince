/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../../testing/mefixture";
import {
  type Company,
  type Company360,
  company,
  emptyPage,
  jsonResponse,
  renderCompany,
  renderSuggestions,
  stub,
  view,
} from "./company360.testkit";

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
});

// The sections these tests build fixtures for, named once. A standalone fixture
// gets no contextual type from `view()`, so its enums widen to `string` and go
// unchecked. That lets a shape drift off the contract while every test passes.
type StateStrip360 = NonNullable<Company360["state_strip"]>;

// The page's two lifecycle controls, header first. They are different
// primitives. The header's is the design system's dropdown worn as a button
// beside the name (a combobox named by its field). The grid's is the inline
// value a row of values carries ("Change Lifecycle"). One of each, so a third
// mount point or a lost one fails here. An account nobody has assessed has an
// empty lifecycle row, which folds behind the details' "Show N empty fields".
async function lifecycleControls(): Promise<HTMLElement[]> {
  const header = await screen.findAllByRole("combobox", { name: "Lifecycle" });
  const unfold = screen.queryByRole("button", {
    name: /^Show \d+ empty fields$/,
  });
  if (unfold) {
    await userEvent.click(unfold);
  }
  const grid = await screen.findAllByRole("button", {
    name: "Change Lifecycle",
  });
  expect(header).toHaveLength(1);
  expect(grid).toHaveLength(1);
  return [...header, ...grid];
}

describe("company view — where the account stands, and what it is to us", () => {
  it("shows where the account stands, and draws no relationship type beside it", async () => {
    // Not "partner". That type also raises the Partner tab, so the type and
    // the tab would share a label and tell the test nothing.
    //
    // The same override goes into both reads: the 360's own `company` (read by
    // the rail's grid) and the standalone GET (read by the header). Both draw a
    // lifecycle control. Two real reads of one row never disagree, so the
    // fixture must not either.
    const withLifecycle: Company = {
      ...company,
      lifecycle: "former_customer",
      relationship_types: ["customer", "supplier"],
    };
    stub(view({ company: withLifecycle }), 200, withLifecycle);
    renderCompany();
    await screen.findByRole("complementary", { name: "Context" });

    // A classification with one value let an account whose contract had ended
    // read as "Prospect" while it was also a partner. Lifecycle is the editable
    // control; the types stay read-only badges. findAllBy, not getBy: the
    // controls appear only once /me answers with the viewer's grants, which
    // resolves apart from the 360 awaited above. Two controls, the header's
    // button and the Details grid's inline field. Both show the same value,
    // because both read the one row and write through the same patch.
    for (const control of await lifecycleControls()) {
      expect(control.textContent).toContain("Former customer");
    }
    // The design draws no relationship badge beside the name and no type row
    // in the details: the types are a Profile tab field. This account is a
    // former customer and still carries the `customer` type. Printing both
    // would read "Former customer" and "Customer" as one fact and its own
    // contradiction.
    expect(screen.queryByText("Customer")).toBeNull();
    expect(screen.queryByText("Supplier")).toBeNull();
  });

  it("offers the lifecycle control on an account nobody has assessed yet", async () => {
    stub(view(), 200, {
      ...company,
      lifecycle: "unknown",
      relationship_types: [],
    });
    renderCompany();
    await screen.findByRole("complementary", { name: "Context" });

    // At 'unknown' a badge would be noise, but a control must still show.
    // Hiding it takes the field away from the account that needs it set, and
    // there is no other way in from here. It reads "Unknown", the stage's own
    // word, never a verdict. Both mount points (header, grid) show it.
    for (const control of await lifecycleControls()) {
      expect(control.textContent).toContain("Unknown");
    }
  });

  it("writes lifecycle once and shows the new value on both mount points", async () => {
    // What "one write path, two mount points" promises: a save through either
    // control reaches the server once. The other control shows the new value
    // once the record refetches, with no second write and no stale value left.
    let currentCompany: Company = { ...company, lifecycle: "unknown" };
    let patchCount = 0;
    let lastIfMatch: string | null = null;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        const pathname = new URL(request.url).pathname;
        if (pathname.endsWith("/360")) {
          return jsonResponse(view({ company: currentCompany }));
        }
        if (pathname.endsWith("/v1/me")) {
          return jsonResponse(
            meFixture({ allow: { company: ["read", "update"] } }),
          );
        }
        if (pathname.endsWith("/companies/o-1")) {
          if (request.method === "PATCH") {
            patchCount += 1;
            lastIfMatch = request.headers.get("if-match");
            // Read as unknown and checked, not asserted. An assertion would let
            // a screen that sent the wrong shape, or nothing, pass as a write
            // of `undefined`. The case below would then assert against a value
            // this stub invented.
            const sent: unknown = await request.json();
            if (sent === null || typeof sent !== "object") {
              throw new Error(`the PATCH body is not an object: ${sent}`);
            }
            const body: { lifecycle?: Company["lifecycle"] } = sent;
            if (currentCompany.version === undefined) {
              // What this case asserts is the If-Match the second write sends,
              // so a fixture with no version to increment would be measuring
              // nothing.
              throw new Error("the account fixture carries no version");
            }
            currentCompany = {
              ...currentCompany,
              lifecycle: body.lifecycle ?? currentCompany.lifecycle,
              version: currentCompany.version + 1,
            };
          }
          return jsonResponse(currentCompany);
        }
        return jsonResponse({ data: [], page: emptyPage });
      }),
    );
    renderCompany();
    await screen.findByRole("complementary", { name: "Context" });

    const [headerControl] = await lifecycleControls();
    await userEvent.click(headerControl);
    await userEvent.click(screen.getByRole("option", { name: "Prospect" }));

    await waitFor(() => expect(patchCount).toBe(1));
    expect(lastIfMatch).toBe("1");
    // Both controls read "Prospect" (the header's own and the grid's reused
    // copy). The single write invalidates the one query both read from; neither
    // wrote a second time.
    await waitFor(async () => {
      for (const control of await lifecycleControls()) {
        expect(control.textContent).toContain("Prospect");
      }
    });
  });
});

// The KPI row's "never render" list is its hard half, and each case below is
// one item of it. They are about what the page must not claim, which a refactor
// loses without a sign.
describe("company view — the KPI row never invents a figure", () => {
  const commercial = (
    over: Partial<NonNullable<StateStrip360["commercial"]>>,
  ): StateStrip360 => ({
    account: { lifecycle: "prospect", relationship_types: [] },
    commercial: {
      open_count: 2,
      stalled_count: 0,
      priced_count: 0,
      converted_count: 0,
      ...over,
    },
  });

  it("shows no money at all when no open deal carries a convertible amount", async () => {
    stub(view({ state_strip: commercial({}) }));
    renderCompany();
    const strip = await screen.findByRole("region", {
      name: "Company status",
    });

    // A zero here would claim a priced pipeline worth nothing. The truth is
    // that the page cannot price this one, so it reports the count instead.
    // No currency figure at all, not merely no zero. A stray non-zero total
    // would be the worse failure, and a looser check would have passed it.
    expect(strip.textContent).not.toMatch(/[€$£]/);
    // The count takes the money's place as the reading, because the count is
    // the fact this account has.
    expect(within(strip).getByText("2")).toBeTruthy();
    expect(within(strip).getByText("No amount")).toBeTruthy();
  });

  it("says an empty pipeline is empty, not unpriced", async () => {
    stub(view({ state_strip: commercial({ open_count: 0 }) }));
    renderCompany();
    const strip = await screen.findByRole("region", {
      name: "Company status",
    });

    // "No amount" on an account with nothing open reports a data problem
    // where the truth is that nothing is running.
    expect(
      within(strip).getByText("Open deals").closest(".stat-card")?.textContent,
    ).toContain("None");
    expect(strip.textContent).not.toContain("No amount");
  });

  it("names the conversion behind a cross-currency total", async () => {
    stub(
      view({
        state_strip: commercial({
          open_pipeline_minor_base: 4500000,
          base_currency: "EUR",
          priced_count: 2,
          converted_count: 1,
          fx_as_of: "2026-02-14",
        }),
      }),
    );
    renderCompany();
    const strip = await screen.findByRole("region", {
      name: "Company status",
    });

    // The KPI rules bar a cross-currency sum with no conversion source and
    // as-of date. The date is the oldest rate behind the figure, which is how
    // far back any part of it reaches. The date itself is checked, not only the
    // prefix: a dropped or wrong interpolation is the failure this
    // qualification exists to prevent.
    expect(within(strip).getByText(/1 converted, rates .*2026/)).toBeTruthy();
  });

  it("keeps saying the pipeline is unpriced even when a deal has stalled", async () => {
    stub(view({ state_strip: commercial({ stalled_count: 1 }) }));
    renderCompany();
    const strip = await screen.findByRole("region", {
      name: "Company status",
    });

    // A reader told only "1 stalled" has no way to know the deals carry no
    // figure at all. Both qualifications are true, so both are shown.
    expect(strip.textContent).toContain("No amount");
    expect(strip.textContent).toContain("1 stalled");
  });

  it("says how much of the pipeline a partial total covers", async () => {
    stub(
      view({
        state_strip: commercial({
          open_pipeline_minor_base: 4500000,
          base_currency: "EUR",
          priced_count: 1,
        }),
      }),
    );
    renderCompany();
    const strip = await screen.findByRole("region", {
      name: "Company status",
    });

    // A sum covering one of two deals, shown bare, reads as the whole of the
    // open work. That is the unlabelled cross-currency total the KPI rules
    // forbid.
    expect(within(strip).getByText("1 of 2 priced")).toBeTruthy();
  });

  it("labels the sum of open deals Open deals, never revenue or potential", async () => {
    stub(
      view({
        state_strip: commercial({
          open_pipeline_minor_base: 4500000,
          base_currency: "EUR",
          priced_count: 2,
        }),
      }),
    );
    renderCompany();
    const strip = await screen.findByRole("region", {
      name: "Company status",
    });

    // Scoped to the card that holds the sum: the money slot beside it is a
    // revenue reading and is entitled to the word.
    const deals = within(strip).getByText("Open deals").closest(".stat-card");
    expect(deals?.textContent).not.toMatch(/revenue|potential/i);
  });

  // Customers and prospects get different questions. A customer's page is asked
  // how it is going with them. A prospect's money card says it has never been
  // billed rather than borrowing a customer's figure. Both rows still carry the
  // account's own standing (relationship and health), which the lifecycle does
  // not get to withhold.
  it("reads never billed on a prospect, and the money itself on a customer", async () => {
    stub(
      view({
        state_strip: {
          account: { lifecycle: "prospect", relationship_types: [] },
          commercial: {
            open_count: 1,
            stalled_count: 0,
            priced_count: 1,
            converted_count: 0,
            open_pipeline_minor_base: 100000,
            base_currency: "EUR",
          },
        },
      }),
    );
    renderCompany();
    let strip = await screen.findByRole("region", {
      name: "Company status",
    });
    expect(within(strip).getByText("Not invoiced")).toBeTruthy();
    expect(within(strip).getByText("Prospect")).toBeTruthy();
    // The strip never carries an overdue cell: what is owed past due is the
    // Payment row's reading and the Finance tab's.
    expect(within(strip).queryByText("Overdue")).toBeNull();
    // A prospect still learns how the relationship stands, in How it stands.
    expect(screen.getAllByText("Relationship").length).toBeGreaterThan(0);

    cleanup();
    stub(
      view({
        health: { days_since_last_inbound: 90 },
        state_strip: {
          account: { lifecycle: "customer", relationship_types: [] },
          commercial: {
            open_count: 1,
            stalled_count: 0,
            priced_count: 1,
            converted_count: 0,
            open_pipeline_minor_base: 100000,
            base_currency: "EUR",
          },
        },
      }),
    );
    renderCompany();
    strip = await screen.findByRole("region", {
      name: "Company status",
    });
    expect(screen.getAllByText("Relationship").length).toBeGreaterThan(0);
    expect(within(strip).queryByText("Overdue")).toBeNull();
    expect(within(strip).queryByText("Not invoiced")).toBeNull();
  });
});

describe("company view — the state strip", () => {
  // Whose move it is and the worst open signal belong to the daily brief
  // (companytoday.tsx). The strip carries the account's standing state only,
  // and those two readings are dated. companytoday.test.tsx covers them; this
  // suite proves the strip does not draw them.
  it("leads with where the account stands and what is open, not whose move it is", async () => {
    stub(
      view({
        state_strip: {
          account: {
            lifecycle: "former_customer",
            relationship_types: ["partner"],
          },
          engagement: {
            state: "waiting_on_them",
            last_inbound_at: "2026-04-30T09:00:00Z",
            last_outbound_at: "2026-07-17T09:00:00Z",
          },
          // The full wire shape, so this case fails if the contract moves
          // under it rather than passing against a loose stub.
          commercial: {
            open_count: 2,
            stalled_count: 1,
            priced_count: 0,
            converted_count: 0,
          },
        },
      }),
    );
    renderCompany();
    await screen.findByRole("region", { name: "Company status" });
    const strip = screen.getByRole("region", {
      name: "Company status",
    });
    expect(within(strip).getByText("2")).toBeTruthy();
    expect(strip.textContent).toContain("1 stalled");
    // Whose move it is reads the same `engagement` field, in the daily brief
    // rather than a second copy in the strip.
    expect(within(strip).queryByText("Awaiting their reply")).toBeNull();
  });

  it("does not draw the worst open signal a second time", async () => {
    stub(
      view({
        state_strip: {
          account: { lifecycle: "prospect", relationship_types: [] },
          engagement: null,
          commercial: null,
          signal: {
            kind: "contract_ended",
            severity: "warn",
            summary: "They wrote that the contract ends on 31 July.",
          },
        },
      }),
    );
    renderCompany();
    const strip = await screen.findByRole("region", {
      name: "Company status",
    });
    // The risk reads the same `state_strip.signal` field, shown in the daily
    // brief (companytoday.test.tsx carries the wording coverage).
    expect(within(strip).queryByText("Contract ending")).toBeNull();
    expect(
      within(strip).queryByText(
        "They wrote that the contract ends on 31 July.",
      ),
    ).toBeNull();
  });
});

describe("company view — advice you can act on", () => {
  it("offers the action the server named, and none where it named none", async () => {
    const three60 = view({
      suggestions: [
        {
          kind: "no_reply",
          fingerprint: "f1",
          reason: "You reached out 15 days ago and nobody has come back.",
          evidence: [],
          action: { kind: "draft_reply", activity_id: "a-1" },
        },
        {
          kind: "no_next_step",
          fingerprint: "f2",
          reason: "2 open deal(s) here and no task saying what happens next.",
          evidence: [],
          action: null,
        },
        // An add_task with no prepared body. The contract promises one on every
        // add_task, so this server broke its word. The answer is still no
        // button, because pressing it would post nothing.
        {
          kind: "no_next_step",
          fingerprint: "f3",
          reason: "3 open deal(s) here and no task saying what happens next.",
          evidence: [],
          action: { kind: "add_task" },
        },
      ],
      suggestions_dropped: 0,
    });
    stub(three60);
    renderSuggestions(three60);
    await screen.findByText(/nobody has come back/);

    expect(screen.getByRole("button", { name: "Create draft" })).toBeTruthy();
    // The second rule named no action, so it advises without a control. A
    // button that does nothing teaches the reader to stop pressing them.
    expect(screen.queryByRole("button", { name: "Add next step" })).toBeNull();
  });

  // The lead panel's footer names what is owed. A count off `data.length` is a
  // claim about the page. So past the 25-row cap the footer says "25+" rather
  // than a number it cannot stand behind.
  it("says how many are overdue at least, when the open-tasks page is capped", async () => {
    const overdueStep = {
      activity_id: "a-1",
      subject: "Send the renewal paperwork",
      due_at: "2026-05-01T09:00:00Z",
      overdue: true,
      linked_deal_id: null,
      linked_contact_id: null,
      assignee_id: null,
    };
    const three60 = view({
      suggestions: [
        {
          kind: "no_reply",
          fingerprint: "f1",
          reason: "You reached out 15 days ago and nobody has come back.",
          evidence: [],
          action: null,
        },
      ],
      next_steps: {
        data: [overdueStep],
        page: { has_more: true, next_cursor: "c1" },
      },
    });
    stub(three60);
    renderSuggestions(three60);

    await waitFor(() => expect(screen.getByText("1+ overdue")).toBeTruthy());
  });
});

// The one money reading the customer row carries: the trailing year. Lifetime,
// open balance, overdue and the payment-habit median are Finance-tab readings
// (companyfinance.test.tsx). This suite pins that the strip stays a glance
// rather than a second copy of that card.
describe("a customer's KPI row reports what the account is worth", () => {
  const customer = {
    account: { lifecycle: "customer" as const, relationship_types: [] },
  };
  const connected = (extra: Record<string, unknown>) => ({
    company_id: "o-1",
    state: "connected",
    provider: "offline_demo",
    ...extra,
  });
  const strip = async () =>
    await screen.findByRole("region", { name: "Company status" });

  // The window the slot names is the window the figure covers. Both figures are
  // on the wire and the lifetime one is larger. A slot that read the wrong
  // field would show a spectacularly good year.
  it("shows the trailing year under the trailing year's label", async () => {
    stub(
      view({ state_strip: customer }),
      200,
      company,
      connected({
        net_invoiced: { amount_minor: 18642000, currency: "EUR" },
        net_invoiced_lifetime: { amount_minor: 42800000, currency: "EUR" },
      }),
    );
    renderCompany();
    const region = await strip();
    // The finance summary is its own query: the slot renders "Loading…" on the
    // first pass, so the assertions below wait for the settled figure.
    await waitFor(() => expect(region.textContent).not.toMatch(/Loading…/));

    expect(within(region).getByText("Revenue · 12 mo")).toBeTruthy();
    // Abbreviated: the strip's slots share its width, and a full euro amount
    // wraps mid-number there. The finance card renders the exact figure.
    expect(within(region).getByText(/186(\.4)?K/i)).toBeTruthy();
    // The lifetime figure appears too, and says which window it covers. This
    // slot must never show it as the trailing year: the lifetime one is larger,
    // so the mistake would read as a spectacularly good year.
    expect(within(region).getByText(/428(\.0)?K lifetime/i)).toBeTruthy();
  });

  // The year's revenue reads off the same finance summary the Finance tab
  // reads, so the two cannot drift. The lifetime total rides in the revenue
  // cell's detail line; what is overdue is the Finance tab's.
  it("reads the year's revenue and leaves what is overdue to the Finance tab", async () => {
    stub(
      view({ state_strip: customer }),
      200,
      company,
      connected({
        net_invoiced: { amount_minor: 18642000, currency: "EUR" },
        net_invoiced_lifetime: { amount_minor: 42800000, currency: "EUR" },
        open_balance: { amount_minor: 3418000, currency: "EUR" },
        overdue: { amount_minor: 1243000, currency: "EUR" },
      }),
    );
    renderCompany();
    const region = await strip();
    await waitFor(() => expect(region.textContent).not.toMatch(/Loading…/));

    expect(within(region).queryByText("Net invoiced · lifetime")).toBeNull();
    expect(within(region).getByText(/186(\.4)?K/i)).toBeTruthy();
    expect(within(region).queryByText("Overdue")).toBeNull();
    expect(within(region).queryByText(/12(\.4)?K/i)).toBeNull();
    expect(within(region).queryByText(/Open .*34(\.\d)?K/i)).toBeNull();
  });
});

// The KPI row's money slot has six reasons it can hold no figure, and they do
// not share a fix. Telling a reader to connect an accounting system they have
// already connected sends them to a settings page to change nothing.
describe("the money slot says WHY it has no figure", () => {
  const customer = {
    account: { lifecycle: "customer" as const, relationship_types: [] },
  };
  const strip = async () =>
    await screen.findByRole("region", { name: "Company status" });

  it("names the setup step only when there is no source", async () => {
    stub(view({ state_strip: customer }), 200, company, {
      company_id: "o-1",
      state: "no_connection",
    });
    renderCompany();
    await strip();
    expect(
      (await screen.findAllByText("Accounting not connected")).length,
    ).toBeGreaterThan(0);
  });

  it("says the source is not matched rather than not connected", async () => {
    stub(view({ state_strip: customer }), 200, company, {
      company_id: "o-1",
      state: "unmapped",
      provider: "offline_demo",
    });
    renderCompany();
    const region = await strip();
    expect(
      (await screen.findAllByText("Customer not matched")).length,
    ).toBeGreaterThan(0);
    // The wrong advice here: this reader has connected a source.
    expect(region.textContent).not.toMatch(/Accounting not connected/);
  });

  it("says a first sync is running rather than that nothing is connected", async () => {
    stub(view({ state_strip: customer }), 200, company, {
      company_id: "o-1",
      state: "syncing",
      provider: "offline_demo",
    });
    renderCompany();
    const region = await strip();
    expect((await screen.findAllByText("Syncing")).length).toBeGreaterThan(0);
    expect(region.textContent).not.toMatch(/Accounting not connected/);
  });

  // A denial and a setup gap are opposite problems. Sending a reader whose role
  // cannot see finance to a settings page asks them to fix what they cannot fix
  // there.
  it("says the reading is withheld rather than telling them to set it up", async () => {
    stub(
      view({ state_strip: customer }),
      200,
      company,
      {
        type: "about:blank",
        title: "Forbidden",
        status: 403,
        code: "permission_denied",
      },
      403,
    );
    renderCompany();
    const region = await strip();
    // The refusal stands as the reading, with no reason under it. The reader
    // can do nothing here, and setup advice would be wrong advice.
    expect((await screen.findAllByText("Restricted")).length).toBeGreaterThan(
      0,
    );
    expect(region.textContent).not.toMatch(/Accounting not connected/);
  });

  it("says the read failed rather than that nothing is connected", async () => {
    stub(
      view({ state_strip: customer }),
      200,
      company,
      { type: "about:blank", title: "Server error", status: 500 },
      500,
    );
    renderCompany();
    const region = await strip();
    expect((await screen.findAllByText("Unavailable")).length).toBeGreaterThan(
      0,
    );
    expect((await screen.findAllByText("Load failed")).length).toBeGreaterThan(
      0,
    );
    expect(region.textContent).not.toMatch(/Accounting not connected/);
  });

  // `stale` and `error` are opposite claims about whether anything is broken.
  // Stale is a sync that succeeded long enough ago that the date matters. Error
  // is the last good answer after an attempt that failed.
  it("calls a stale figure old, not failed", async () => {
    stub(view({ state_strip: customer }), 200, company, {
      company_id: "o-1",
      state: "stale",
      provider: "offline_demo",
      net_invoiced: { amount_minor: 18642000, currency: "EUR" },
    });
    renderCompany();
    await strip();
    expect((await screen.findAllByText(/186\.4k/i)).length).toBeGreaterThan(0);
    // Beside a figure the caveat qualifies the figure: this one is the last
    // good number and may have moved since.
    expect((await screen.findAllByText("Not current")).length).toBeGreaterThan(
      0,
    );
    expect(screen.queryByText(/sync failed/i)).toBeNull();
  });

  // Without this the last good figure renders bare, reading as current.
  it("marks a figure from a failed sync as possibly not current", async () => {
    stub(view({ state_strip: customer }), 200, company, {
      company_id: "o-1",
      state: "error",
      provider: "offline_demo",
      net_invoiced: { amount_minor: 18642000, currency: "EUR" },
    });
    renderCompany();
    await strip();
    expect((await screen.findAllByText(/186\.4k/i)).length).toBeGreaterThan(0);
    expect(
      (await screen.findAllByText(/Last sync failed/)).length,
    ).toBeGreaterThan(0);
  });

  // A live, mapped source that produced no figure is not a missing setup.
  it("says nothing was invoiced rather than telling them to connect", async () => {
    stub(view({ state_strip: customer }), 200, company, {
      company_id: "o-1",
      state: "connected",
      provider: "offline_demo",
    });
    renderCompany();
    const region = await strip();
    // The same word a never-billed prospect gets, kept apart from it by the
    // lifecycle in the detail.
    expect((await screen.findAllByText("Not invoiced")).length).toBeGreaterThan(
      0,
    );
    expect((await screen.findAllByText("Customer")).length).toBeGreaterThan(0);
    expect(region.textContent).not.toMatch(/Accounting not connected/);
  });

  it("names the source beside a real figure", async () => {
    stub(view({ state_strip: customer }), 200, company, {
      company_id: "o-1",
      state: "connected",
      provider: "datev",
      net_invoiced: { amount_minor: 18642000, currency: "EUR" },
    });
    renderCompany();
    await strip();
    expect((await screen.findAllByText(/186\.4k/i)).length).toBeGreaterThan(0);
    expect((await screen.findAllByText("datev")).length).toBeGreaterThan(0);
  });
});

// The money slot's reason is said once, because there is one slot to say it in.
// A figure from a window the slot does not name never stands in for one it
// does.
describe("the money slot says its reason once and borrows no figure", () => {
  const customer = {
    account: { lifecycle: "customer" as const, relationship_types: [] },
  };
  const strip = async () =>
    await screen.findByRole("region", { name: "Company status" });

  it("says why there is no figure once, not once per money window", async () => {
    stub(view({ state_strip: customer }), 200, company, {
      company_id: "o-1",
      state: "unmapped",
      provider: "offline_demo",
    });
    renderCompany();
    const region = await strip();
    await waitFor(() =>
      expect(within(region).getAllByText("Customer not matched").length).toBe(
        1,
      ),
    );

    // One statement, not one per cell: the overdue cell beside it has no
    // figure either, and leaves the reason to the revenue cell.
    expect(within(region).queryByText("Net invoiced · lifetime")).toBeNull();
  });

  // The lifetime total is the figure most likely present when the trailing year
  // is not: an account billed years ago and nothing since. Under the trailing
  // year's label it would report a year that never happened, so the slot
  // refuses and says why.
  it("refuses rather than showing a lifetime total as the trailing year", async () => {
    stub(view({ state_strip: customer }), 200, company, {
      company_id: "o-1",
      state: "unmapped",
      provider: "offline_demo",
      net_invoiced_lifetime: { amount_minor: 42800000, currency: "EUR" },
    });
    renderCompany();
    const region = await strip();
    await waitFor(() => expect(region.textContent).not.toMatch(/Loading…/));

    // No figure at all, and the reason named. The lifetime total on the wire is
    // not this slot's answer, and a dash with no reason would not be either.
    expect(region.textContent).not.toMatch(/428(\.0)?K/i);
    expect(within(region).getAllByText("Customer not matched").length).toBe(1);
  });

  // Two shapes: a customer whose finance connection cannot answer, and one
  // whose connection can. Both must still carry the account's standing
  // (pipeline, relationship); the defect guarded is the standing disappearing
  // behind the money.
  const withStanding = () =>
    view({
      state_strip: {
        account: { lifecycle: "customer" as const, relationship_types: [] },
        commercial: {
          open_count: 2,
          open_pipeline_minor_base: 500000,
          base_currency: "EUR",
          stalled_count: 0,
          priced_count: 2,
          converted_count: 2,
        },
      },
      health: {
        days_since_last_inbound: 5,
        reply_balance: 0.5,
        relationship: { rating: "strong", reason: "Active both ways" },
      },
    });

  it("keeps the account's standing readings beside an unanswerable Finance slot", async () => {
    stub(withStanding(), 200, company, {
      company_id: "o-1",
      state: "unmapped",
      provider: "offline_demo",
    });
    renderCompany();
    const region = await strip();
    await waitFor(() =>
      expect(within(region).getAllByText("Customer not matched").length).toBe(
        1,
      ),
    );

    expect(within(region).getByText("Open deals")).toBeTruthy();
    // The relationship reads in How it stands above the figures, whatever the
    // money cells could or could not read.
    expect(screen.getAllByText("Relationship").length).toBeGreaterThan(0);
    expect(within(region).getByText("Last contact")).toBeTruthy();
    expect(within(region).queryByText("Overdue")).toBeNull();
    // One label on the money slot whatever it can report, so a reader never has
    // to notice the slot renamed itself.
    expect(within(region).getByText("Revenue · 12 mo")).toBeTruthy();
  });

  it("shows the real figure beside the same standing readings", async () => {
    stub(withStanding(), 200, company, {
      company_id: "o-1",
      state: "connected",
      provider: "datev",
      net_invoiced: { amount_minor: 18642000, currency: "EUR" },
      net_invoiced_lifetime: { amount_minor: 42800000, currency: "EUR" },
      overdue: { amount_minor: 1243000, currency: "EUR" },
    });
    renderCompany();
    const region = await strip();
    await waitFor(() => expect(region.textContent).not.toMatch(/Loading…/));

    expect(within(region).getByText("Open deals")).toBeTruthy();
    expect(screen.getAllByText("Relationship").length).toBeGreaterThan(0);
    expect(within(region).getByText("Last contact")).toBeTruthy();
    expect(within(region).queryByText("Overdue")).toBeNull();
    expect(within(region).getByText("Revenue · 12 mo")).toBeTruthy();
    expect(within(region).getByText(/186(\.4)?K/i)).toBeTruthy();
    // The slot never renames itself: "Finance" was the label it wore with no
    // figure, and a label that moves is a reading read twice.
    expect(within(region).queryByText("Finance")).toBeNull();
  });
});
