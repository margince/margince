/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import { en } from "../../i18n/en";
import {
  company,
  emptyPage,
  jsonResponse,
  renderRail,
  stub,
  view,
} from "./companyrail.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("CompanyRail", () => {
  it("draws one row per contact, naming the colleagues already in touch with them", () => {
    stub();
    renderRail({
      view: view({
        contacts: {
          data: [
            {
              contact_id: "p-1",
              full_name: "Dana Buyer",
              title: "VP Procurement",
              strength: {
                score: 40,
                bucket: "moderate",
                factors: {},
                inbound_90d: 1,
              },
              deal_roles: [],
              consent: {},
              routes: {
                top: [
                  {
                    user_id: "u-2",
                    display_name: "Ravi Shah",
                    strength_bucket: "strong",
                  },
                ],
                remainder: 0,
                untried: false,
              },
            },
          ],
          page: emptyPage,
        },
      }),
    });
    expect(screen.getByText("Dana Buyer")).toBeInTheDocument();
    expect(screen.getByText("VP Procurement")).toBeInTheDocument();
    // Named again for anyone not reading the stacked avatars as monograms:
    // the sr-only text beside them, not the face itself.
    expect(screen.getByText("Ravi Shah")).toBeInTheDocument();
    // The rail names contacts it cannot say anything more about; the row is the
    // way to the record that can.
    expect(
      screen.getByRole("link", { name: "Dana Buyer" }).getAttribute("href"),
    ).toBe("#/contacts/p-1");
  });

  it("shows only the top RAIL_ROW_LIMIT deals, an attention-carrying one among them, while the header names the full count", () => {
    stub();
    renderRail({
      view: view({
        deals: {
          data: [
            {
              deal_id: "d-1",
              name: "Fleet renewal",
              status: "open",
              stage_name: "Negotiation",
              amount: { amount_minor: 480_000, currency: "EUR" },
              stalled: false,
            },
            {
              deal_id: "d-2",
              name: "Spare parts contract",
              status: "open",
              stage_name: "Discovery",
              amount: { amount_minor: 200_000, currency: "EUR" },
              stalled: false,
            },
            {
              deal_id: "d-3",
              name: "Service contract",
              status: "open",
              stage_name: "Proposal",
              amount: { amount_minor: 150_000, currency: "EUR" },
              stalled: false,
            },
            // The lowest amount of the five, but carries an attention flag —
            // the rail's own ranking puts it ahead of every deal that has
            // nothing pressing about it, amount aside.
            {
              deal_id: "d-4",
              name: "Support renewal",
              status: "open",
              stage_name: "Negotiation",
              amount: { amount_minor: 10_000, currency: "EUR" },
              stalled: false,
              attention: {
                kind: "overdue_task",
                title: "Send updated quote",
                who: "Mira Voss",
                due_at: "2026-05-01T00:00:00Z",
              },
            },
            {
              deal_id: "d-5",
              name: "Onboarding add-on",
              status: "open",
              stage_name: "Discovery",
              amount: { amount_minor: 50_000, currency: "EUR" },
              stalled: false,
            },
          ],
          page: emptyPage,
          won_lifetime: { amount_minor: 0, currency: null },
          lost_count: 0,
        },
      }),
    });
    const dealsPanel = screen
      .getByRole("heading", { name: "Active deals" })
      .closest<HTMLElement>("details");
    expect(dealsPanel).not.toBeNull();
    if (!dealsPanel) {
      throw new Error("the deals panel has no wrapper");
    }
    // The attention-carrying deal first, then the two largest amounts of the
    // remaining four, all in one currency so the tiebreak is deterministic —
    // a regression in byDealWeight's amount compare must fail this, not just
    // the count.
    expect(within(dealsPanel).getAllByRole("link").length).toBe(3);
    expect(within(dealsPanel).getByText("Support renewal")).toBeInTheDocument();
    expect(within(dealsPanel).getByText("Fleet renewal")).toBeInTheDocument();
    expect(
      within(dealsPanel).getByText("Spare parts contract"),
    ).toBeInTheDocument();
    expect(
      within(dealsPanel).queryByText("Service contract"),
    ).not.toBeInTheDocument();
    expect(
      within(dealsPanel).queryByText("Onboarding add-on"),
    ).not.toBeInTheDocument();
    expect(
      within(dealsPanel).getByRole("button", { name: "All 5" }),
    ).toBeInTheDocument();
  });

  it("keeps the server's order past the attention split when the amounts span currencies", () => {
    stub();
    renderRail({
      view: view({
        deals: {
          data: [
            {
              deal_id: "d-1",
              // The SMALLER raw minor-unit figure stands first in the
              // server's order, so a naive cross-currency amount sort would
              // move Tokyo above it — which is exactly the regression this
              // fixture exists to catch. Yen minor units dwarf euro ones on
              // digit count alone; ranked honestly the server's order stands.
              name: "Berlin expansion",
              status: "open",
              stage_name: "Discovery",
              amount: { amount_minor: 100_000, currency: "EUR" },
              stalled: false,
            },
            {
              deal_id: "d-2",
              name: "Tokyo rollout",
              status: "open",
              stage_name: "Proposal",
              amount: { amount_minor: 900_000, currency: "JPY" },
              stalled: false,
            },
            {
              deal_id: "d-3",
              name: "Support renewal",
              status: "open",
              stage_name: "Renewal",
              amount: { amount_minor: 50_000, currency: "EUR" },
              stalled: true,
            },
          ],
          page: emptyPage,
          won_lifetime: { amount_minor: 0, currency: null },
          lost_count: 0,
        },
      }),
    });
    const dealsPanel = screen
      .getByText("Active deals")
      .closest("details, .panel");
    if (!(dealsPanel instanceof HTMLElement)) {
      throw new Error("the deals panel has no wrapper");
    }
    // The stalled deal leads on attention; behind it the two clean deals keep
    // the order the server sent, because a mixed-currency amount compare is
    // no compare at all.
    const names = within(dealsPanel)
      .getAllByRole("link")
      .map((link) => link.querySelector(".record-card-name")?.textContent);
    expect(names).toEqual([
      "Support renewal",
      "Berlin expansion",
      "Tokyo rollout",
    ]);
  });

  it("lets priced deals rank when the only other currency sits on an unpriced deal", () => {
    stub();
    renderRail({
      view: view({
        deals: {
          data: [
            {
              deal_id: "d-1",
              name: "Berlin expansion",
              status: "open",
              stage_name: "Discovery",
              amount: { amount_minor: 100_000, currency: "EUR" },
              stalled: false,
            },
            {
              deal_id: "d-2",
              // A currency with no figure behind it: nothing to rank, so it
              // must not stop the two priced EUR deals from ranking.
              name: "US exploratory",
              status: "open",
              stage_name: "Discovery",
              amount: { amount_minor: null, currency: "USD" },
              stalled: false,
            },
            {
              deal_id: "d-3",
              name: "Fleet renewal",
              status: "open",
              stage_name: "Negotiation",
              amount: { amount_minor: 480_000, currency: "EUR" },
              stalled: false,
            },
          ],
          page: emptyPage,
          won_lifetime: { amount_minor: 0, currency: null },
          lost_count: 0,
        },
      }),
    });
    const dealsPanel = screen
      .getByText("Active deals")
      .closest("details, .panel");
    if (!(dealsPanel instanceof HTMLElement)) {
      throw new Error("the deals panel has no wrapper");
    }
    const names = within(dealsPanel)
      .getAllByRole("link")
      .map((link) => link.querySelector(".record-card-name")?.textContent);
    // The larger EUR figure leads; the unpriced deal ranks after the priced.
    expect(names).toEqual([
      "Fleet renewal",
      "Berlin expansion",
      "US exploratory",
    ]);
  });

  it("shows only the top RAIL_ROW_LIMIT contacts while the header names the full count", () => {
    stub();
    renderRail({
      view: view({
        contacts: {
          data: [
            { contact_id: "p-1", full_name: "Dana Buyer" },
            { contact_id: "p-2", full_name: "Erik Voss" },
            { contact_id: "p-3", full_name: "Farah Lund" },
            { contact_id: "p-4", full_name: "Gita Reyes" },
            { contact_id: "p-5", full_name: "Hugo Berg" },
          ].map((contact) => ({
            ...contact,
            deal_roles: [],
            strength: {
              score: 40,
              bucket: "moderate",
              factors: {},
              inbound_90d: 1,
            },
          })),
          page: emptyPage,
        },
      }),
    });
    const contactsPanel = screen
      .getByRole("heading", { name: "Key contacts" })
      .closest<HTMLElement>("details");
    expect(contactsPanel).not.toBeNull();
    if (!contactsPanel) {
      throw new Error("the contacts panel has no wrapper");
    }
    expect(
      within(contactsPanel).getAllByRole("link", {
        name: /Buyer|Voss|Lund|Reyes|Berg/,
      }).length,
    ).toBe(3);
    expect(
      within(contactsPanel).getByRole("button", { name: "All 5" }),
    ).toBeInTheDocument();
  });

  it("draws one row per deal, with its amount and a stage · close-date note", () => {
    stub();
    renderRail({
      view: view({
        deals: {
          data: [
            {
              deal_id: "d-1",
              name: "Fleet renewal",
              status: "open",
              stage_name: "Negotiation",
              amount: { amount_minor: 480_000, currency: "EUR" },
              expected_close_date: "2026-07-01",
              stalled: false,
            },
            {
              deal_id: "d-2",
              name: "Spare parts contract",
              status: "open",
              stage_name: "Discovery",
              amount: { amount_minor: null, currency: null },
              expected_close_date: null,
              stalled: false,
            },
          ],
          page: emptyPage,
          won_lifetime: { amount_minor: 0, currency: null },
          lost_count: 0,
        },
      }),
    });
    expect(screen.getByText("Fleet renewal")).toBeInTheDocument();
    expect(screen.getByText("€4,800.00")).toBeInTheDocument();
    expect(
      screen.getByText("Negotiation · closes 01/07/2026"),
    ).toBeInTheDocument();
    expect(screen.getByText("Discovery · no close date")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Fleet renewal" }).getAttribute("href"),
    ).toBe("#/deals/d-1");
  });

  it("prefixes the note with the deal's own attention ahead of a bare stall flag", () => {
    stub();
    renderRail({
      view: view({
        deals: {
          data: [
            {
              deal_id: "d-1",
              name: "Fleet renewal",
              status: "open",
              stage_name: "Negotiation",
              amount: { amount_minor: 480_000, currency: "EUR" },
              expected_close_date: null,
              stalled: true,
              attention: {
                kind: "overdue_task",
                title: "Send updated quote",
                who: "Mira Voss",
                due_at: "2026-05-01T00:00:00Z",
              },
            },
          ],
          page: emptyPage,
          won_lifetime: { amount_minor: 0, currency: null },
          lost_count: 0,
        },
      }),
    });
    expect(
      screen.getByText("Overdue · Negotiation · no close date"),
    ).toBeInTheDocument();
  });

  it("switches to the Deals tab from the header's own All-N link", async () => {
    const spy = vi.fn();
    stub();
    renderRail({
      view: view({
        deals: {
          data: [
            {
              deal_id: "d-1",
              name: "Fleet renewal",
              status: "open",
              stage_name: "Negotiation",
              stalled: false,
            },
          ],
          page: emptyPage,
          won_lifetime: { amount_minor: 0, currency: null },
          lost_count: 0,
        },
      }),
      onTab: spy,
    });
    await userEvent.click(screen.getByRole("button", { name: "All 1" }));
    expect(spy).toHaveBeenCalledWith("deals");
  });

  it("offers to create the account's first deal when it has never had one", async () => {
    stub({
      "/pipelines": () =>
        jsonResponse({
          data: [
            {
              id: "pl-1",
              name: "Default",
              is_default: true,
              stages: [{ id: "s-1", name: "Discovery", semantic: "open" }],
            },
          ],
          page: emptyPage,
        }),
    });
    renderRail();
    expect(
      screen.getByText("No deals for this company yet."),
    ).toBeInTheDocument();
    expect(
      await screen.findByRole("button", { name: "New deal" }),
    ).toBeInTheDocument();
    // The create verb is the empty pipeline's ONE verb: no second "Add"
    // beside it offering the same tab under another name.
    const dealsPanel = screen
      .getByRole("heading", { name: "Active deals" })
      .closest<HTMLElement>("details");
    expect(dealsPanel).not.toBeNull();
    expect(
      dealsPanel && within(dealsPanel).queryByRole("button", { name: "Add" }),
    ).toBeNull();
  });

  it("reads a closed-only pipeline as nothing open rather than never started", () => {
    stub();
    renderRail({
      view: view({
        deals: {
          data: [],
          page: emptyPage,
          won_lifetime: { amount_minor: 250_000, currency: "EUR" },
          lost_count: 1,
        },
      }),
    });
    expect(
      screen.getByText("No open deals, only closed ones."),
    ).toBeInTheDocument();
    // No first-deal verb here — the account has already had deals, it is
    // between two of them rather than never having started. The way to the
    // Deals tab stands in its place, as the section's one verb.
    expect(
      screen.queryByRole("button", { name: "New deal" }),
    ).not.toBeInTheDocument();
    const dealsPanel = screen
      .getByRole("heading", { name: "Active deals" })
      .closest<HTMLElement>("details");
    expect(dealsPanel).not.toBeNull();
    expect(
      dealsPanel && within(dealsPanel).getByRole("button", { name: "Add" }),
    ).toBeInTheDocument();
  });

  it("offers to add the account's first contact when it has none", async () => {
    const spy = vi.fn();
    stub();
    renderRail({ onTab: spy });
    expect(screen.getByText(en["co.rail.contacts.empty"])).toBeInTheDocument();
    await userEvent.click(
      screen.getByRole("button", { name: en["co.rail.contacts.add"] }),
    );
    expect(spy).toHaveBeenCalledWith("contacts");
    // That is the empty roster's one verb: no second "Add" under it.
    const contactsPanel = screen
      .getByRole("heading", { name: "Key contacts" })
      .closest<HTMLElement>("details");
    expect(contactsPanel).not.toBeNull();
    expect(
      contactsPanel &&
        within(contactsPanel).queryByRole("button", { name: "Add" }),
    ).toBeNull();
  });

  it("drops the count once the server has cut the page, on contacts and deals alike", () => {
    stub();
    const contacts = Array.from({ length: 25 }, (_, i) => ({
      contact_id: `p-${i + 1}`,
      full_name: `Contact ${i + 1}`,
      deal_roles: [],
      strength: { score: 40, bucket: "moderate", factors: {}, inbound_90d: 1 },
    }));
    const deals = Array.from({ length: 25 }, (_, i) => ({
      deal_id: `d-${i + 1}`,
      name: `Deal ${i + 1}`,
      status: "open",
      stage_name: "Discovery",
      amount: { amount_minor: 1_000 * (i + 1), currency: "EUR" },
      stalled: false,
    }));
    renderRail({
      view: view({
        contacts: {
          data: contacts,
          page: { has_more: true, next_cursor: "c" },
        },
        deals: {
          data: deals,
          page: { has_more: true, next_cursor: "c" },
          won_lifetime: { amount_minor: 0, currency: null },
          lost_count: 0,
        },
      }),
    });
    // Twenty-five is where the server cut, not how many there are: the
    // summary carries no badge and the verb no figure, on both sections.
    for (const name of ["Key contacts", "Active deals"]) {
      const panel = screen
        .getByRole("heading", { name })
        .closest<HTMLElement>("details");
      expect(panel).not.toBeNull();
      if (!panel) {
        throw new Error(`${name} has no wrapper`);
      }
      expect(within(panel).queryByText("25")).not.toBeInTheDocument();
      expect(
        within(panel).getByRole("button", { name: "All" }),
      ).toBeInTheDocument();
      expect(
        within(panel).queryByRole("button", { name: /All \d/ }),
      ).not.toBeInTheDocument();
    }
  });

  it("offers the add-tag verb once the section has answered, on a writable record", async () => {
    stub();
    renderRail();
    // The section answered `empty` (view()'s tags defaults to []), so the
    // verb renders beside the tags it acts on.
    expect(
      await screen.findByRole("button", { name: /add tag/i }),
    ).toBeInTheDocument();
  });

  it("withholds the add-tag verb on an archived record", async () => {
    stub();
    renderRail({
      view: view({
        company: { ...company, archived_at: "2026-06-02T00:00:00Z" },
      }),
    });
    // The values themselves still draw (this suite's own empty-badges test
    // covers that); only the verb stands down.
    await waitFor(() => {
      expect(
        screen.queryByRole("button", { name: /add tag/i }),
      ).not.toBeInTheDocument();
    });
  });

  // sectionState (company360.tsx) reads an undefined `view` as "loading" only
  // when told the composite read is still running — otherwise it reads the
  // exact same undefined `view` as "unavailable", the words a real outage
  // shows. Before `loading` was threaded through, every ordinary page open
  // flashed "could not be loaded" on Health/Contacts/Tags for as long as the
  // read was in flight. Pinned here as two DIFFERENT renders rather than one
  // happy-path check, because a fix that only makes the loading case not
  // crash is not the fix — it has to render DIFFERENTLY from the failed case.
  it("reads an in-flight composite read as loading, not unavailable", () => {
    stub();
    const { container } = renderRail({ view: undefined, loading: true });
    // The skeleton placeholder, not the "could not be loaded" sentence.
    expect(container.querySelector(".skeleton")).toBeTruthy();
    expect(screen.queryByText(en["state.unavailable"])).not.toBeInTheDocument();
  });

  it("reads a failed composite read (view undefined, not loading) as unavailable", () => {
    stub();
    renderRail({ view: undefined, loading: false });
    // The SAME undefined `view` as the loading test above, but with
    // `loading={false}` — the honest "could not be loaded" sentence, not a
    // skeleton pretending a read is still running.
    expect(screen.getAllByText(en["state.unavailable"]).length).toBeGreaterThan(
      0,
    );
  });
});
