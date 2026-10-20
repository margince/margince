// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import { cleanup, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "@testing-library/jest-dom/vitest";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { en } from "../../i18n/en";
import { ContactsSection } from "./companyrail";
import {
  emptyPage,
  jsonResponse,
  onTab,
  render,
  stub,
  view,
} from "./companyrail.testkit";
import { DealsSection } from "./companyraildeals";

type Company360 = components["schemas"]["Company360"];
type Tab = "deals" | "contacts";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The overview's Deals and Contacts sections, as the page mounts them.
function renderSections({
  view: three60 = view(),
  onTab: onOpenTab = onTab,
}: Readonly<{ view?: Company360; onTab?: (tab: Tab) => void }> = {}) {
  return render(
    <>
      <DealsSection view={three60} loading={false} onTab={onOpenTab} />
      <ContactsSection view={three60} loading={false} onTab={onOpenTab} />
    </>,
  );
}

// The section whose title starts with the name. The title carries a count
// beside it, so the heading's whole name is not the title alone.
function section(kind: Tab): HTMLElement {
  const title = kind === "deals" ? "Deals" : "Contacts";
  const panel = screen
    .getAllByRole("heading")
    .find((heading) => heading.textContent?.startsWith(title))
    ?.closest<HTMLElement>(".panel");
  if (!panel) {
    throw new Error(`the ${title} section is not drawn`);
  }
  return panel;
}

describe("the overview's Deals and Contacts sections", () => {
  it("draws one row per contact, naming the colleagues already in touch with them", () => {
    stub();
    renderSections({
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
    renderSections({
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
            // The lowest amount of the five, but it carries an attention flag.
            // The ranking puts it ahead of every deal with nothing pressing.
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
          won_count: 0,
        },
      }),
    });
    const dealsPanel = section("deals");
    expect(dealsPanel).not.toBeNull();
    if (!dealsPanel) {
      throw new Error("the deals panel has no wrapper");
    }
    // The flagged deal first, then the two largest amounts of the other four.
    // All share one currency, so a broken amount compare fails this too.
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
    expect(within(dealsPanel).getByText("5 open")).toBeInTheDocument();
    expect(
      within(dealsPanel).getByRole("button", { name: "All deals" }),
    ).toBeInTheDocument();
  });

  it("keeps the server's order past the attention split when the amounts span currencies", () => {
    stub();
    renderSections({
      view: view({
        deals: {
          data: [
            {
              deal_id: "d-1",
              // The smaller raw minor-unit figure stands first in the server's
              // order. A cross-currency amount sort would move Tokyo above it,
              // because yen minor units dwarf euro ones on digit count alone.
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
          won_count: 0,
        },
      }),
    });
    const dealsPanel = section("deals");
    if (!(dealsPanel instanceof HTMLElement)) {
      throw new Error("the deals panel has no wrapper");
    }
    // The stalled deal leads on attention. The two clean deals keep the server's
    // order behind it: a mixed-currency amount compare is no compare at all.
    const names = within(dealsPanel)
      .getAllByRole("link")
      .map((link) => link.textContent);
    expect(names).toEqual([
      "Support renewal",
      "Berlin expansion",
      "Tokyo rollout",
    ]);
  });

  it("lets priced deals rank when the only other currency sits on an unpriced deal", () => {
    stub();
    renderSections({
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
          won_count: 0,
        },
      }),
    });
    const dealsPanel = section("deals");
    if (!(dealsPanel instanceof HTMLElement)) {
      throw new Error("the deals panel has no wrapper");
    }
    const names = within(dealsPanel)
      .getAllByRole("link")
      .map((link) => link.textContent);
    // The larger EUR figure leads; the unpriced deal ranks after the priced.
    expect(names).toEqual([
      "Fleet renewal",
      "Berlin expansion",
      "US exploratory",
    ]);
  });

  it("shows only the top RAIL_ROW_LIMIT contacts while the header names the full count", () => {
    stub();
    renderSections({
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
    const contactsPanel = section("contacts");
    expect(contactsPanel).not.toBeNull();
    if (!contactsPanel) {
      throw new Error("the contacts panel has no wrapper");
    }
    expect(
      within(contactsPanel).getAllByRole("link", {
        name: /Buyer|Voss|Lund|Reyes|Berg/,
      }).length,
    ).toBe(3);
    expect(within(contactsPanel).getByText("5")).toBeInTheDocument();
    expect(
      within(contactsPanel).getByRole("button", { name: "All contacts" }),
    ).toBeInTheDocument();
  });

  it("draws one row per deal, with its amount, its stage and a close-date note", () => {
    stub();
    renderSections({
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
          won_count: 0,
        },
      }),
    });
    expect(screen.getByText("Fleet renewal")).toBeInTheDocument();
    expect(screen.getByText("€4,800.00")).toBeInTheDocument();
    expect(screen.getByText("Negotiation")).toBeInTheDocument();
    expect(screen.getByText("closes 01/07/2026")).toBeInTheDocument();
    expect(screen.getByText("no close date")).toBeInTheDocument();
    expect(
      screen.getByRole("link", { name: "Fleet renewal" }).getAttribute("href"),
    ).toBe("#/deals/d-1");
  });

  it("prefixes the note with the deal's own attention ahead of a bare stall flag", () => {
    stub();
    renderSections({
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
          won_count: 0,
        },
      }),
    });
    expect(screen.getByText("Overdue · no close date")).toBeInTheDocument();
  });

  it("switches to the Deals tab from the header's own All-deals link", async () => {
    const spy = vi.fn();
    stub();
    renderSections({
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
          won_count: 0,
        },
      }),
      onTab: spy,
    });
    await userEvent.click(screen.getByRole("button", { name: "All deals" }));
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
    renderSections();
    expect(
      screen.getByText("No deals for this company yet."),
    ).toBeInTheDocument();
    expect(
      await screen.findByRole("button", { name: "New deal" }),
    ).toBeInTheDocument();
    // The create verb is the empty pipeline's one verb: no second "Add"
    // beside it offering the same tab under another name.
    const dealsPanel = section("deals");
    expect(dealsPanel).not.toBeNull();
    expect(
      dealsPanel && within(dealsPanel).queryByRole("button", { name: "Add" }),
    ).toBeNull();
  });

  it("reads a closed-only pipeline as nothing open rather than never started", () => {
    stub();
    renderSections({
      view: view({
        deals: {
          data: [],
          page: emptyPage,
          won_lifetime: { amount_minor: 250_000, currency: "EUR" },
          lost_count: 1,
          won_count: 0,
        },
      }),
    });
    expect(
      screen.getByText("No open deals, only closed ones."),
    ).toBeInTheDocument();
    // No first-deal verb here: the account is between deals, not new. The way
    // to the Deals tab stands in its place, as the section's one verb.
    expect(
      screen.queryByRole("button", { name: "New deal" }),
    ).not.toBeInTheDocument();
    const dealsPanel = section("deals");
    expect(dealsPanel).not.toBeNull();
    expect(
      dealsPanel && within(dealsPanel).getByRole("button", { name: "Add" }),
    ).toBeInTheDocument();
  });

  it("offers to add the account's first contact when it has none", async () => {
    const spy = vi.fn();
    stub();
    renderSections({ onTab: spy });
    expect(screen.getByText(en["co.rail.contacts.empty"])).toBeInTheDocument();
    await userEvent.click(
      screen.getByRole("button", { name: en["co.rail.contacts.add"] }),
    );
    expect(spy).toHaveBeenCalledWith("contacts");
    // That is the empty roster's one verb: no second "Add" under it.
    const contactsPanel = section("contacts");
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
    renderSections({
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
          won_count: 0,
        },
      }),
    });
    // Twenty-five is where the server cut, not how many there are: the
    // heading carries no figure on either section, and its link still opens.
    const sections: [Tab, string][] = [
      ["contacts", "Contacts"],
      ["deals", "Deals"],
    ];
    for (const [kind, title] of sections) {
      expect(within(section(kind)).getByRole("heading").textContent).toBe(
        title,
      );
      expect(
        within(section(kind)).getByRole("button", { name: `All ${kind}` }),
      ).toBeInTheDocument();
    }
  });

  it("marks a withheld section restricted instead of drawing it empty", () => {
    stub();
    renderSections({ view: view({ sections_omitted: ["contacts"] }) });
    expect(screen.getAllByText("Hidden for your role").length).toBeGreaterThan(
      0,
    );
    // A withheld section carries no header link. A count would have nothing
    // true to show, and an "Add" would write into a section the reader cannot
    // see. Scoped to Contacts: Deals shows its own "Add" when empty.
    expect(
      within(section("contacts")).queryByRole("button", {
        name: /^All contacts$|^Add$/,
      }),
    ).toBeNull();
  });
});
