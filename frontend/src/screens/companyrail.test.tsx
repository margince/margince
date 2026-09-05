/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "@testing-library/jest-dom/vitest";
import type { ComponentProps, ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { CompanyRail } from "./companyrail";

type Organization360 = components["schemas"]["Organization360"];

// The rail's own honesty rules: a section the caller's role withheld says so
// rather than drawing the empty state that would read as "there is none",
// and a field the record does not carry still draws its row — an unfilled
// field is a fact worth showing, not one this grid hides.

const org = {
  // Absent reads as NOT writable, which is the fail-closed default a real
  // response never relies on: the server answers this per row.
  writable: true,
  id: "o-1",
  workspace_id: "w",
  display_name: "Brandt Automotive GmbH",
  legal_name: "Brandt Automotive GmbH",
  lifecycle: "customer" as const,
  owner_id: "u-1",
  industry: "Automotive",
  size_band: "51-200" as const,
  linkedin_url: "https://linkedin.com/company/brandt",
  address: { city: "Munich", country: "DE" },
  domains: [{ domain: "brandt.example", is_primary: true, source: "manual" }],
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

const emptyPage = { has_more: false, next_cursor: null };

// No test in this file asserts on where a header link sends the reader —
// that is organizations.test.tsx's own claim, since the callback only makes
// sense wired to the real tab strip it switches.
const onTab = () => {};

// Built loosely and cast once here, matching company360.test.tsx's own
// fixture: a hand-typed 360 payload restates the generated schema by hand,
// and the two would silently drift the moment the contract grows a field
// this suite never needed.
function view(overrides: Record<string, unknown> = {}): Organization360 {
  return {
    as_of: "2026-06-01T09:00:00Z",
    organization: org,
    sections_omitted: [],
    people: { data: [], page: emptyPage },
    deals: {
      data: [],
      page: emptyPage,
      won_lifetime: { amount_minor: 0, currency: null },
      lost_count: 0,
    },
    tags: [],
    ...overrides,
  } as unknown as Organization360;
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{ui}</LocaleProvider>
    </QueryClientProvider>,
  );
}

type RailProps = ComponentProps<typeof CompanyRail>;

// Every site below wants the same rail: a writable view, not loading,
// composer closed, and a no-op tab switch. `overrides` supplies whatever the
// test is actually varying.
function renderRail(overrides: Partial<RailProps> = {}) {
  return render(
    <CompanyRail
      orgId="o-1"
      view={view()}
      loading={false}
      composerOpen={false}
      onTab={onTab}
      {...overrides}
    />,
  );
}

// The one path every test needs, whatever it is asserting: the finance
// summary Health reads for its payment dimension, the roster the owner
// row resolves against, and the signals feed. `overrides` answers with
// whatever the test is actually about.
function stub(
  overrides: Record<
    string,
    (req: Request) => Response | Promise<Response>
  > = {},
) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const pathname = new URL(request.url).pathname;
      for (const [suffix, respond] of Object.entries(overrides)) {
        if (pathname.endsWith(suffix)) {
          return await respond(request);
        }
      }
      if (pathname.endsWith("/finance-summary")) {
        return jsonResponse({ organization_id: "o-1", state: "no_connection" });
      }
      if (pathname.endsWith("/users")) {
        return jsonResponse({
          data: [{ id: "u-1", display_name: "Mira Voss" }],
          page: emptyPage,
        });
      }
      // A real seat, spelled out. The catch-all below answers `/me` with an
      // empty page, which `useCan` reads as a principal holding NO grants — so
      // every control gated on one stood down and the suite proved nothing
      // about the controls it names.
      if (pathname.endsWith("/me")) {
        return jsonResponse(
          meFixture({ allow: { organization: ["read", "update"] } }),
        );
      }
      if (pathname.endsWith("/signals")) {
        return jsonResponse({ data: [], page: emptyPage });
      }
      return jsonResponse({ data: [], page: emptyPage });
    }),
  );
}

describe("CompanyRail", () => {
  it("renders nothing while the composer holds the column", () => {
    stub();
    renderRail({ composerOpen: true });
    expect(screen.queryByText("Details")).not.toBeInTheDocument();
  });

  it("draws the details grid from the fields the record actually carries", async () => {
    stub();
    renderRail();
    expect(screen.getByText("Brandt Automotive GmbH")).toBeInTheDocument();
    expect(screen.getByText("Automotive")).toBeInTheDocument();
    expect(screen.getByText("51-200")).toBeInTheDocument();
    // Address draws one row per part now rather than one combined "Munich, DE"
    // summary.
    expect(screen.getByText("Munich")).toBeInTheDocument();
    expect(screen.getByText("DE")).toBeInTheDocument();
    expect(screen.getByText("brandt.example")).toBeInTheDocument();
    // The owner cell resolves through the roster read, same as EntityRef
    // does everywhere else: not shown until the read lands.
    await waitFor(() =>
      expect(screen.getByText("Mira Voss")).toBeInTheDocument(),
    );
  });

  it("still draws every known row when the record carries no value for it", () => {
    stub();
    const bare = {
      ...org,
      legal_name: null,
      industry: null,
      size_band: null,
      linkedin_url: null,
      address: undefined,
      domains: [],
      owner_id: null,
    };
    renderRail({ view: view({ organization: bare }) });
    // Every row's LABEL still draws: an absent field is a fact about the
    // record, not a reason to hide the row that would say so. Both Industry
    // (InlineText, which draws no label of its own) and Company size
    // (InlineChoice with `hideLabel`, which suppresses its own visible
    // "label: value") get their visible label from FieldRow's own label
    // column and no other node — checked directly since neither wraps its
    // label into a combined string anymore. Address is six rows now, one per
    // part; City stands in for the other five.
    expect(screen.getByText("Industry")).toBeInTheDocument();
    expect(screen.getByText("Company size")).toBeInTheDocument();
    expect(screen.getByText("City")).toBeInTheDocument();
    // No /me grant in this stub, so every field renders its read-only
    // fallback rather than a control — owner and every address part share
    // the same "Not set"/"Unassigned" absence text the grid always used.
    expect(screen.getByText("Unassigned")).toBeInTheDocument();
    expect(screen.getAllByText("Not set").length).toBeGreaterThan(0);
  });

  it("opens and saves the LinkedIn URL inline even when one is already set", async () => {
    let patchBody: unknown;
    stub({
      "/me": () =>
        jsonResponse({
          user: { id: "u-1", display_name: "Mira Voss" },
          authorization: {
            objects: { organization: { update: true } },
            // A full seat: the licensing ceiling is checked before RBAC, and the
            // grid's controls issue a PATCH.
            seat_type: "full",
          },
        }),
      "/organizations/o-1": async (request) => {
        if (request.method === "PATCH") {
          patchBody = await request.json();
          return jsonResponse({ ...org, version: 2 });
        }
        return jsonResponse(org);
      },
    });
    renderRail();
    // The value is already set, which is exactly the case that used to render
    // a bare link with no control in any branch — there was nothing to open.
    // Waits for /me's grant to resolve first: the button exists only once
    // `canEdit` turns true, same as every other inline control here.
    await userEvent.click(
      await screen.findByRole("button", { name: "Change LinkedIn URL" }),
    );
    const input = screen.getByLabelText("LinkedIn URL");
    await userEvent.clear(input);
    // No Save button in the edit-in-place rework: Enter is the commit.
    await userEvent.type(
      input,
      "https://linkedin.com/company/brandt-gmbh{Enter}",
    );
    await waitFor(() =>
      expect(patchBody).toMatchObject({
        linkedin_url: "https://linkedin.com/company/brandt-gmbh",
      }),
    );
  });

  it("edits owner through the same roster-backed control the header uses", async () => {
    let patchBody: unknown;
    stub({
      "/me": () =>
        jsonResponse({
          user: { id: "u-1", display_name: "Mira Voss" },
          authorization: {
            objects: { organization: { update: true } },
            // A full seat: the licensing ceiling is checked before RBAC, and the
            // grid's controls issue a PATCH.
            seat_type: "full",
          },
        }),
      "/users": () =>
        jsonResponse({
          data: [
            { id: "u-1", display_name: "Mira Voss" },
            { id: "u-2", display_name: "Ravi Shah" },
          ],
          page: emptyPage,
        }),
      "/organizations/o-1": async (request) => {
        if (request.method === "PATCH") {
          patchBody = await request.json();
          return jsonResponse({ ...org, version: 2 });
        }
        return jsonResponse(org);
      },
    });
    renderRail();
    await waitFor(() =>
      expect(screen.getByText("Mira Voss")).toBeInTheDocument(),
    );
    // The click that starts editing already opens the picker — no second
    // click on the combobox first.
    await userEvent.click(screen.getByRole("button", { name: "Change Owner" }));
    await userEvent.click(screen.getByRole("option", { name: "Ravi Shah" }));
    await waitFor(() => expect(patchBody).toMatchObject({ owner_id: "u-2" }));
  });

  it("surfaces the server's refusal on the owner control rather than swallowing it", async () => {
    // A stale roster entry (a user removed between page load and save) is the
    // one way an accepted-looking choice still fails: the wire FK on
    // organization.owner_id (core 0019) rejects it as a reference the server
    // cannot resolve, and the rail's owner control must show that sentence
    // next to itself — the same generic refusal path InlineChoice already
    // proves in design-system/inlinechoice.test.tsx, exercised here through
    // the shared control this grid now reuses.
    stub({
      "/me": () =>
        jsonResponse({
          user: { id: "u-1", display_name: "Mira Voss" },
          authorization: {
            objects: { organization: { update: true } },
            // A full seat: the licensing ceiling is checked before RBAC, and the
            // grid's controls issue a PATCH.
            seat_type: "full",
          },
        }),
      "/users": () =>
        jsonResponse({
          data: [
            { id: "u-1", display_name: "Mira Voss" },
            { id: "u-2", display_name: "Ravi Shah" },
          ],
          page: emptyPage,
        }),
      "/organizations/o-1": (request) => {
        if (request.method === "PATCH") {
          return jsonResponse(
            {
              type: "about:blank",
              title: "Unprocessable Entity",
              status: 422,
              code: "reference_not_found",
              detail: "The referenced record was not found.",
            },
            422,
          );
        }
        return jsonResponse(org);
      },
    });
    renderRail();
    await waitFor(() =>
      expect(screen.getByText("Mira Voss")).toBeInTheDocument(),
    );
    await userEvent.click(screen.getByRole("button", { name: "Change Owner" }));
    await userEvent.click(screen.getByRole("option", { name: "Ravi Shah" }));
    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toContain(
        "The referenced record was not found.",
      ),
    );
    // The picker stays open on the reader's choice rather than snapping back
    // to the old owner, the same rule InlineChoice keeps for every field.
    expect(screen.getByRole("combobox")).toBeInTheDocument();
  });

  it("edits lifecycle in the grid through the header's own control", async () => {
    // Only the rail renders in this suite — the header's copy of this same
    // control (and the "one implementation, two mount points" claim that
    // depends on both being on screen at once) is company360.test.tsx's own
    // to prove, since that is where both actually mount together.
    let patchBody: unknown;
    stub({
      "/me": () =>
        jsonResponse({
          user: { id: "u-1", display_name: "Mira Voss" },
          authorization: {
            objects: { organization: { update: true } },
            // A full seat: the licensing ceiling is checked before RBAC, and the
            // grid's controls issue a PATCH.
            seat_type: "full",
          },
        }),
      "/organizations/o-1": async (request) => {
        if (request.method === "PATCH") {
          patchBody = await request.json();
          return jsonResponse({ ...org, version: 2 });
        }
        return jsonResponse(org);
      },
    });
    renderRail();
    await userEvent.click(
      await screen.findByRole("button", { name: "Change Account lifecycle" }),
    );
    // The fixture is already "customer" — picking a DIFFERENT value, or the
    // no-op guard skips the write entirely.
    await userEvent.click(screen.getByRole("option", { name: "Prospect" }));
    await waitFor(() =>
      expect(patchBody).toMatchObject({ lifecycle: "prospect" }),
    );
  });

  it("renames the primary domain while preserving every other domain on the account", async () => {
    let patchBody: unknown;
    const threeDomains = {
      ...org,
      domains: [
        { domain: "brandt.example", is_primary: true, source: "manual" },
        { domain: "brandt.de", is_primary: false, source: "manual" },
        {
          domain: "brandt-automotive.com",
          is_primary: false,
          source: "manual",
        },
      ],
    };
    stub({
      "/me": () =>
        jsonResponse({
          user: { id: "u-1", display_name: "Mira Voss" },
          authorization: {
            objects: { organization: { update: true } },
            // A full seat: the licensing ceiling is checked before RBAC, and the
            // grid's controls issue a PATCH.
            seat_type: "full",
          },
        }),
      "/organizations/o-1": async (request) => {
        if (request.method === "PATCH") {
          patchBody = await request.json();
          return jsonResponse({ ...threeDomains, version: 2 });
        }
        return jsonResponse(threeDomains);
      },
    });
    renderRail({ view: view({ organization: threeDomains }) });
    await userEvent.click(
      await screen.findByRole("button", { name: "Change Domain" }),
    );
    const input = screen.getByLabelText("Domain");
    await userEvent.clear(input);
    await userEvent.type(input, "brandt-gmbh.example{Enter}");
    await waitFor(() => expect(patchBody).toBeTruthy());
    // The other two domains survive untouched, and only the renamed primary
    // changed — sending just the edited entry would have silently dropped
    // them, exactly the replace-set trap this row exists to avoid.
    expect(patchBody).toMatchObject({
      domains: expect.arrayContaining([
        expect.objectContaining({ domain: "brandt.de", is_primary: false }),
        expect.objectContaining({
          domain: "brandt-automotive.com",
          is_primary: false,
        }),
        expect.objectContaining({
          domain: "brandt-gmbh.example",
          is_primary: true,
        }),
      ]),
    });
    expect((patchBody as { domains: unknown[] }).domains).toHaveLength(3);
  });

  it("refuses to clear the domain field rather than deleting the primary domain", async () => {
    const onSave = vi.fn();
    stub({
      "/me": () =>
        jsonResponse({
          user: { id: "u-1", display_name: "Mira Voss" },
          authorization: {
            objects: { organization: { update: true } },
            // A full seat: the licensing ceiling is checked before RBAC, and the
            // grid's controls issue a PATCH.
            seat_type: "full",
          },
        }),
      "/organizations/o-1": async (request) => {
        if (request.method === "PATCH") {
          onSave(await request.json());
        }
        return jsonResponse(org);
      },
    });
    renderRail();
    await userEvent.click(
      await screen.findByRole("button", { name: "Change Domain" }),
    );
    const input = screen.getByLabelText("Domain");
    await userEvent.clear(input);
    await userEvent.keyboard("{Enter}");
    await waitFor(() =>
      expect(screen.getByRole("alert").textContent).toContain(
        "cannot be cleared here",
      ),
    );
    expect(onSave).not.toHaveBeenCalled();
  });

  it("edits an address part by sending the whole address back with only that part changed", async () => {
    let patchBody: unknown;
    stub({
      "/me": () =>
        jsonResponse({
          user: { id: "u-1", display_name: "Mira Voss" },
          authorization: {
            objects: { organization: { update: true } },
            // A full seat: the licensing ceiling is checked before RBAC, and the
            // grid's controls issue a PATCH.
            seat_type: "full",
          },
        }),
      "/organizations/o-1": async (request) => {
        if (request.method === "PATCH") {
          patchBody = await request.json();
          return jsonResponse({ ...org, version: 2 });
        }
        return jsonResponse(org);
      },
    });
    renderRail();
    await userEvent.click(
      await screen.findByRole("button", { name: "Change City" }),
    );
    const input = screen.getByLabelText("City");
    await userEvent.clear(input);
    await userEvent.type(input, "Stuttgart{Enter}");
    await waitFor(() =>
      expect(patchBody).toMatchObject({
        // `country` survives from the record's existing address even though
        // this edit never touched it — a write that omitted it would have
        // blanked it, since `address` replaces the object wholesale.
        address: { city: "Stuttgart", country: "DE" },
      }),
    );
  });

  it("marks a withheld section restricted instead of drawing it empty", () => {
    stub();
    renderRail({ view: view({ sections_omitted: ["people"] }) });
    expect(
      screen.getAllByText("Hidden — your role cannot read this").length,
    ).toBeGreaterThan(0);
    // A withheld section carries no header link: a count would have nothing
    // true to show, and an "Add" would offer to write into a section the
    // reader cannot even see. Scoped to People's own panel — Deals is
    // unrelated and legitimately shows its own "Add" for its own empty read.
    const peoplePanel = screen
      .getByRole("heading", { name: "Their key people" })
      .closest<HTMLElement>("details");
    expect(peoplePanel).not.toBeNull();
    expect(
      peoplePanel &&
        within(peoplePanel).queryByRole("button", { name: /All \d|^Add$/ }),
    ).toBeNull();
  });

  it("draws one row per contact, naming the colleagues already in touch with them", () => {
    stub();
    renderRail({
      view: view({
        people: {
          data: [
            {
              person_id: "p-1",
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
    // The rail names people it cannot say anything more about; the row is the
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
      .map((link) => link.textContent);
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
    renderRail({
      view: view({
        people: {
          data: [
            { person_id: "p-1", full_name: "Dana Buyer" },
            { person_id: "p-2", full_name: "Erik Voss" },
            { person_id: "p-3", full_name: "Farah Lund" },
            { person_id: "p-4", full_name: "Gita Reyes" },
            { person_id: "p-5", full_name: "Hugo Berg" },
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
    const peoplePanel = screen
      .getByRole("heading", { name: "Their key people" })
      .closest<HTMLElement>("details");
    expect(peoplePanel).not.toBeNull();
    if (!peoplePanel) {
      throw new Error("the people panel has no wrapper");
    }
    expect(
      within(peoplePanel).getAllByRole("link", {
        name: /Buyer|Voss|Lund|Reyes|Berg/,
      }).length,
    ).toBe(3);
    expect(
      within(peoplePanel).getByRole("button", { name: "All 5" }),
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
      screen.getByText("No deals on this account yet."),
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
      screen.getByText("Nothing open — only closed history."),
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
    expect(screen.getByText(en["co.rail.people.empty"])).toBeInTheDocument();
    await userEvent.click(
      screen.getByRole("button", { name: en["co.rail.people.add"] }),
    );
    expect(spy).toHaveBeenCalledWith("people");
    // That is the empty roster's one verb: no second "Add" under it.
    const peoplePanel = screen
      .getByRole("heading", { name: "Their key people" })
      .closest<HTMLElement>("details");
    expect(peoplePanel).not.toBeNull();
    expect(
      peoplePanel && within(peoplePanel).queryByRole("button", { name: "Add" }),
    ).toBeNull();
  });

  it("drops the count once the server has cut the page, on people and deals alike", () => {
    stub();
    const contacts = Array.from({ length: 25 }, (_, i) => ({
      person_id: `p-${i + 1}`,
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
        people: { data: contacts, page: { has_more: true, next_cursor: "c" } },
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
    for (const name of ["Their key people", "Active deals"]) {
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
        organization: { ...org, archived_at: "2026-06-02T00:00:00Z" },
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
  // flashed "could not be loaded" on Health/People/Tags for as long as the
  // read was in flight. Pinned here as two DIFFERENT renders rather than one
  // happy-path check, because a fix that only makes the loading case not
  // crash is not the fix — it has to render DIFFERENTLY from the failed case.
  it("reads an in-flight composite read as loading, not unavailable", () => {
    stub();
    const { container } = renderRail({ view: undefined, loading: true });
    // The skeleton placeholder, not the "could not be loaded" sentence.
    expect(container.querySelector(".skeleton")).toBeTruthy();
    expect(
      screen.queryByText(
        "Could not be loaded — this may not be the whole picture",
      ),
    ).not.toBeInTheDocument();
  });

  it("reads a failed composite read (view undefined, not loading) as unavailable", () => {
    stub();
    renderRail({ view: undefined, loading: false });
    // The SAME undefined `view` as the loading test above, but with
    // `loading={false}` — the honest "could not be loaded" sentence, not a
    // skeleton pretending a read is still running.
    expect(
      screen.getAllByText(
        "Could not be loaded — this may not be the whole picture",
      ).length,
    ).toBeGreaterThan(0);
  });
});
