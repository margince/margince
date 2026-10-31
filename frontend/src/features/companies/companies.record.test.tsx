/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
  cleanup,
  render as rtlRender,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider, translate } from "../../i18n";
import {
  company,
  company360,
  companyBackstop,
  emptyPage,
  jsonResponse,
} from "../../testing/company.fixtures";
import { stubFetch } from "../../testing/company.testkit";
import { CompanyScreen } from "./companies";
import { openProfile, openRecordMenu, render } from "./companies.testkit";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.location.hash = "";
});

// openHistory switches to the tab the account's chronology lives on. The
// timeline is not on the overview, so a test about it has to go there first.
async function openHistory() {
  await userEvent.click(
    await screen.findByRole("button", { name: /^History/ }),
  );
}

const employmentRel = {
  id: "rel-1",
  kind: "employment",
  contact_id: "p-1",
  company_id: "o-1",
  role: "cto",
  is_current_primary: true,
  started_at: "2024-01-01",
  ended_at: null,
  source: "manual",
  captured_by: "human:u1",
  version: 1,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const rollup = {
  root_id: "o-1",
  scope: "tree",
  weighted_pipeline: { amount_minor: 4_800_000, currency: "EUR" },
  closed_won: { amount_minor: 1_200_000, currency: "EUR" },
  activity_count_30d: 12,
  aggregated_account_count: 3,
  restricted_excluded: [],
  computed_at: "2026-07-01T09:30:00Z",
  pipeline_state_at: "2026-07-01T09:30:00Z",
  closed_won_period: {
    grain: "quarter",
    label: "2026-Q3",
    start_at: "2026-07-01T00:00:00Z",
    end_at: "2026-10-01T00:00:00Z",
    anchor: "closed_at",
    chosen: false,
    previous: "2026-Q2",
    next: null,
  },
  activity_window: {
    start_at: "2026-07-01T09:30:00Z",
    end_at: "2026-07-01T09:30:00Z",
  },
  current_periods: {
    period_year: "2026",
    period_quarter: "2026-Q3",
    period_month: "2026-07",
  },
};

describe("CompanyScreen — Relationships tab (P-5)", () => {
  it("shows an Overview/Relationships tab bar and lists relationships by company_id", async () => {
    stubFetch(async (url) => {
      if (url.includes("/relationships") && url.includes("company_id=o-1")) {
        return jsonResponse({
          data: [employmentRel],
          page: { next_cursor: null, has_more: false },
        });
      }
      if (url.includes("/activities")) {
        return jsonResponse({ data: [] });
      }
      return jsonResponse(company);
    });
    render(<CompanyScreen id="o-1" />);
    await openProfile();

    await waitFor(() => expect(screen.getByText("Employment")).toBeTruthy());
    expect(screen.getByText("cto")).toBeTruthy();
    expect(screen.getByText("p-1")).toBeTruthy();
  });

  it("adding a relationship from the company side POSTs company_id + the picked contact_id", async () => {
    let posted: unknown = null;
    stubFetch(async (url, method, request) => {
      if (method === "POST" && url.includes("/relationships")) {
        posted = JSON.parse(await request.text());
        return jsonResponse({ ...employmentRel, id: "rel-new" }, 201);
      }
      if (url.includes("/relationships") && url.includes("company_id=o-1")) {
        return emptyPage();
      }
      if (url.includes("/contacts?") && url.includes("q=anna")) {
        return jsonResponse({
          data: [{ id: "p-1", full_name: "Anna Weber" }],
          page: { next_cursor: null, has_more: false },
        });
      }
      if (url.includes("/activities")) {
        return jsonResponse({ data: [] });
      }
      return jsonResponse(company);
    });
    render(<CompanyScreen id="o-1" />);
    await openProfile();
    await waitFor(() =>
      expect(screen.getByTestId("add-relationship")).toBeTruthy(),
    );
    await userEvent.click(screen.getByTestId("add-relationship"));

    await userEvent.type(screen.getByPlaceholderText("Search…"), "anna");
    vi.useFakeTimers();
    try {
      act(() => {
        vi.advanceTimersByTime(250);
      });
    } finally {
      vi.useRealTimers();
    }
    await waitFor(() => expect(screen.getByText("Anna Weber")).toBeTruthy());
    await userEvent.click(screen.getByText("Anna Weber"));
    await userEvent.click(screen.getByTestId("add-relationship-submit"));

    await waitFor(() => expect(posted).toBeTruthy());
    expect(posted).toMatchObject({
      company_id: "o-1",
      contact_id: "p-1",
      kind: "employment",
      source: "manual",
    });
  });
});

describe("CompanyScreen — hierarchy roll-up in the rail (P-7)", () => {
  it("shows the weighted pipeline, closed-won, activity, and account figures", async () => {
    stubFetch(
      async (url) => {
        if (url.includes("/activities")) {
          return jsonResponse({ data: [] });
        }
        return jsonResponse(company);
      },
      { rollup },
    );
    render(<CompanyScreen id="o-1" />);
    await openProfile();

    await waitFor(() => expect(screen.getByText("€48,000.00")).toBeTruthy());
    expect(screen.getByText("€12,000.00")).toBeTruthy();
    expect(screen.getByText("12")).toBeTruthy();
    expect(screen.getByText("3")).toBeTruthy();
  });

  it("renders the honest FX-unavailable message instead of zeros on a 422", async () => {
    stubFetch(
      async (url) => {
        if (url.includes("/activities")) {
          return jsonResponse({ data: [] });
        }
        return jsonResponse(company);
      },
      {
        rollup: jsonResponse(
          { title: "Unprocessable", code: "fx_rate_unavailable" },
          422,
        ),
      },
    );
    render(<CompanyScreen id="o-1" />);
    await openProfile();

    await waitFor(() =>
      expect(
        screen.getByText(
          "An exchange rate is missing, so the total cannot be calculated.",
        ),
      ).toBeTruthy(),
    );
    expect(screen.queryByText("€0.00")).toBeNull();
  });

  it.each([
    [1, "1 hidden company excluded"],
    [2, "2 hidden companies excluded"],
  ])(
    "discloses the hidden companies the viewer cannot read (%i)",
    async (hidden, said) => {
      stubFetch(
        async (url) => {
          if (url.includes("/activities")) {
            return jsonResponse({ data: [] });
          }
          return jsonResponse(company);
        },
        {
          rollup: {
            ...rollup,
            restricted_excluded: Array.from({ length: hidden }, (_, i) => ({
              id: `o-9${i}`,
              display_name: "Hidden Subsidiary GmbH",
            })),
          },
        },
      );
      render(<CompanyScreen id="o-1" />);
      await openProfile();

      await waitFor(() => expect(screen.getByText(said)).toBeTruthy());
    },
  );
});

describe("CompanyScreen — the account pulse line (P-4)", () => {
  it("names the way in and when they last spoke, and shows no composite score", async () => {
    stubFetch(
      async (url) => {
        if (url.includes("/activities")) {
          return jsonResponse({ data: [] });
        }
        if (url.includes("/contacts/p-1")) {
          return jsonResponse({
            ...company,
            id: "p-1",
            full_name: "Dana Buyer",
          });
        }
        return jsonResponse(company);
      },
      {
        company360: {
          ...company360,
          strength: {
            score: 41,
            bucket: "weak",
            contact_count: 3,
            contributor_contact_id: "p-1",
            factors: {
              recency: 0.3,
              frequency: 0.2,
              reciprocity: 0.4,
              direction: 0.5,
            },
            last_interaction: "2026-06-20T12:00:00Z",
          },
          last_inbound_at: "2026-06-20T12:00:00Z",
          last_outbound_at: "2026-06-28T09:00:00Z",
        },
      },
    );
    render(<CompanyScreen id="o-1" />);

    // The way in. When contact last happened is the readings row's (Last
    // contact), not the header's: one fact, one home.
    await waitFor(() => expect(screen.getByText(/Best route/)).toBeTruthy());
    expect(screen.getByText(/of 3 contacts here/)).toBeTruthy();
    const facts = screen.getByText(/Best route/).closest(".record-split-head");
    if (!(facts instanceof HTMLElement)) {
      throw new Error("the way in is not drawn among the head's lines");
    }
    expect(within(facts).queryByText(/Last contact/)).toBeNull();
    // No composite score. A MAX over contacts lets one talkative contact speak
    // for the account, and "41/100" reads as a verdict.
    expect(screen.queryByText(/41\/100/)).toBeNull();
  });

  it("says there is no relationship rather than showing a zero", async () => {
    stubFetch(async (url) => {
      if (url.includes("/activities")) {
        return jsonResponse({ data: [] });
      }
      return jsonResponse(company);
    });
    render(<CompanyScreen id="o-1" />);

    await waitFor(() =>
      expect(screen.getByText("Brandt Automotive GmbH")).toBeTruthy(),
    );
    // company360's backstop omits `strength`, as for an account with no
    // readable contacts. No way in is named, and no score stands in for one.
    expect(screen.queryByText(/Best route/)).toBeNull();
    expect(screen.queryByText(/^0 ·/)).toBeNull();
  });
});

describe("CompanyScreen — archived is read-only (P-3)", () => {
  it("hides edit/merge/archive and shows the Archived badge on an archived company", async () => {
    stubFetch(async (url) => {
      if (url.includes("/activities")) {
        return jsonResponse({ data: [] });
      }
      return jsonResponse({ ...company, archived_at: "2026-07-13T00:00:00Z" });
    });
    render(<CompanyScreen id="o-1" />);

    await waitFor(() => expect(screen.getByText("Archived")).toBeTruthy());
    expect(screen.queryByTestId("edit-record")).toBeNull();
    expect(screen.queryByTestId("merge-record")).toBeNull();
    expect(screen.queryByTestId("archive-record")).toBeNull();
  });
});

describe("CompanyScreen — relationship kinds by scope (P-5)", () => {
  it("offers company↔company kinds (not deal_stakeholder) from a company and POSTs counterparty_company_id", async () => {
    const user = userEvent.setup();
    let posted: unknown = null;
    stubFetch(async (url, method, request) => {
      if (method === "POST" && url.includes("/relationships")) {
        posted = JSON.parse(await request.text());
        return jsonResponse({ ...employmentRel, id: "rel-new" }, 201);
      }
      if (url.includes("/relationships") && url.includes("company_id=o-1")) {
        return emptyPage();
      }
      if (url.includes("/companies?") && url.includes("q=acme")) {
        return jsonResponse({
          data: [{ id: "o-2", display_name: "Acme Corp" }],
          page: { next_cursor: null, has_more: false },
        });
      }
      if (url.includes("/activities")) {
        return jsonResponse({ data: [] });
      }
      return jsonResponse(company);
    });
    render(<CompanyScreen id="o-1" />);
    await user.click(await screen.findByRole("button", { name: "Profile" }));
    await waitFor(() =>
      expect(screen.getByTestId("add-relationship")).toBeTruthy(),
    );
    await user.click(screen.getByTestId("add-relationship"));

    // A company anchors employment + the company↔company kinds; deal_stakeholder needs a
    // contact endpoint and must not be offered here. The kinds only exist in the
    // DOM while the popup is open, so the absence is asserted on an open list.
    const kind = screen.getByLabelText("Kind");
    await user.click(kind);
    const kinds = screen.getByRole("listbox");
    expect(within(kinds).queryByText("Deal stakeholder")).toBeNull();
    await user.click(within(kinds).getByRole("option", { name: "Partner of" }));

    await user.type(screen.getByPlaceholderText("Search…"), "acme");
    vi.useFakeTimers();
    try {
      act(() => {
        vi.advanceTimersByTime(250);
      });
    } finally {
      vi.useRealTimers();
    }
    await waitFor(() => expect(screen.getByText("Acme Corp")).toBeTruthy());
    await user.click(screen.getByText("Acme Corp"));
    await user.click(screen.getByTestId("add-relationship-submit"));

    await waitFor(() => expect(posted).toBeTruthy());
    expect(posted).toMatchObject({
      company_id: "o-1",
      counterparty_company_id: "o-2",
      kind: "partner_of",
      source: "manual",
    });
    expect(posted).not.toHaveProperty("contact_id");
  });
});

describe("CompanyScreen — the header's overflow menu", () => {
  // The panel keeps its items mounted so a dialog opened from one survives the
  // click that closes the menu. So "closed" is asserted on the panel, not on
  // the absence of the items. An author `display` rule can beat the UA's
  // `[hidden] {display:none}` and leave every destructive verb open.
  it("keeps its items out of the page until the trigger is used", async () => {
    stubFetch(companyBackstop);
    render(<CompanyScreen id="o-1" />);

    const trigger = await screen.findByRole("button", { name: "More actions" });
    expect(trigger.getAttribute("aria-expanded")).toBe("false");
    const panelId = trigger.getAttribute("aria-controls");
    expect(panelId).toBeTruthy();
    const panel = document.getElementById(panelId ?? "");
    expect(panel?.hasAttribute("hidden")).toBe(true);

    await userEvent.click(trigger);

    expect(trigger.getAttribute("aria-expanded")).toBe("true");
    expect(panel?.hasAttribute("hidden")).toBe(false);
  });
});

describe("CompanyScreen — the record's history", () => {
  // The audit spine is an inspection of the record, not part of the account's
  // story. So it opens from the header's overflow menu, not as a tab.
  it("opens the full history from the overflow menu", async () => {
    stubFetch(async (url) => {
      if (url.includes("/records/company/o-1/history")) {
        return jsonResponse({
          data: [
            {
              id: "h1",
              actor_type: "human",
              actor_id: "u1",
              actor_name: "Demo Admin",
              action: "create",
              occurred_at: "2026-07-13T10:00:00Z",
              summary: "Created the record",
            },
          ],
          page: { next_cursor: null },
        });
      }
      if (url.includes("/activities")) {
        return jsonResponse({ data: [] });
      }
      return jsonResponse(company);
    });
    render(<CompanyScreen id="o-1" />);

    await userEvent.click(await openRecordMenu("company-full-history"));

    await screen.findByText(
      translate("en", "history.verb.create", { subject: "Demo Admin" }),
    );
  });

  // Field changes are the account's own chronology, so they sit in the
  // timeline behind a filter, not on a screen of their own.
  it("draws the Changes cut as the chronicle's own change rows, without the restore control", async () => {
    stubFetch(async (url) => {
      if (/\/records\/[^/]+\/[^/]+\/history/.test(url)) {
        return jsonResponse({
          data: [
            {
              id: "a1",
              actor_type: "human",
              actor_id: "u1",
              actor_name: "Demo Admin",
              action: "update",
              occurred_at: "2026-07-14T10:00:00Z",
              summary: "Demo Admin updated the record",
              before: { industry: "Automotive" },
              after: { industry: "Manufacturing" },
              undoable: { undoable: true },
            },
          ],
          page: { next_cursor: null, has_more: false },
        });
      }
      if (url.includes("/field-history")) {
        return jsonResponse({
          data: [
            {
              id: "f1",
              entity_type: "company",
              entity_id: "o-1",
              field: "industry",
              old_value: "Automotive",
              new_value: "Manufacturing",
              changed_at: "2026-07-14T10:00:00Z",
              actor_type: "human",
              actor_id: "u1",
            },
          ],
          page: { next_cursor: null },
        });
      }
      return jsonResponse(company);
    });
    render(<CompanyScreen id="o-1" />);
    await openHistory();

    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Changes" })).toBeTruthy(),
    );
    await userEvent.click(screen.getByRole("button", { name: "Changes" }));

    // Scoped to the timeline: the account is called "Brandt Automotive GmbH",
    // so a page-wide match on the old value would pass on the heading.
    const timeline = await screen.findByRole("region", { name: "History" });
    await waitFor(() =>
      expect(within(timeline).getByText("Manufacturing")).toBeTruthy(),
    );
    expect(within(timeline).getByText("Automotive")).toBeTruthy();
    expect(within(timeline).getByText("Industry")).toBeTruthy();
    // The Changes cut draws the same chronicle rows the All view interleaves,
    // with no second rendering of the audit rows. It has no restore control:
    // put-back lives on Full history (the header's overflow menu) only.
    expect(within(timeline).queryByRole("button", { name: "Undo" })).toBeNull();
  });
});

// The 360 caps its activities section. A capped list that says nothing reads
// as the whole history: the rep takes the oldest row on screen for the day the
// relationship started.
describe("CompanyScreen — the timeline says where it stops", () => {
  it("says the activity list is cut when the 360 reports more", async () => {
    stubFetch(companyBackstop, {
      company360: {
        ...company360,
        activities: {
          data: [
            {
              id: "a-1",
              kind: "email",
              subject: "Re: Lead Gen",
              occurred_at: "2026-06-01T08:30:00Z",
              direction: "inbound",
            },
          ],
          page: { has_more: true, next_cursor: "c1" },
        },
      },
    });
    render(<CompanyScreen id="o-1" />);
    await openHistory();

    await waitFor(() =>
      expect(screen.getAllByText("Re: Lead Gen").length).toBeGreaterThan(0),
    );
    expect(
      screen.getByText("Only the most recent activities are shown."),
    ).toBeTruthy();
  });

  it("stays silent when the account's whole activity list is on screen", async () => {
    stubFetch(companyBackstop, {
      company360: {
        ...company360,
        activities: {
          data: [
            {
              id: "a-1",
              kind: "email",
              subject: "Re: Lead Gen",
              occurred_at: "2026-06-01T08:30:00Z",
              direction: "inbound",
            },
          ],
          page: { has_more: false, next_cursor: null },
        },
      },
    });
    render(<CompanyScreen id="o-1" />);
    await openHistory();

    await waitFor(() =>
      expect(screen.getAllByText("Re: Lead Gen").length).toBeGreaterThan(0),
    );
    expect(screen.queryByText(/more activities than fit here/)).toBeNull();
  });
});

// The filter belongs to the account being read, not to the session. The route
// swaps one company for another without unmounting, so a filter kept in the
// component would follow the reader to every account.
describe("CompanyScreen — the timeline filter does not follow you", () => {
  it("returns to All when another company is opened", async () => {
    stubFetch(async (url) =>
      /\/companies\/o-\d$/.test(new URL(url).pathname)
        ? jsonResponse(company)
        : emptyPage(),
    );
    // One QueryClient across both renders, with both records already cached.
    // That keeps the record component mounted across the swap. A cold second
    // record would unmount it and reset the filter for free.
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    client.setQueryData(["company", "o-1"], company);
    client.setQueryData(["company", "o-2"], { ...company, id: "o-2" });
    const page = (id: string) => (
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <CompanyScreen id={id} />
        </LocaleProvider>
      </QueryClientProvider>
    );
    const { rerender } = rtlRender(page("o-1"));
    await openHistory();

    const pressed = (name: string) =>
      screen.getByRole("button", { name }).getAttribute("aria-pressed");

    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Changes" })).toBeTruthy(),
    );
    await userEvent.click(screen.getByRole("button", { name: "Changes" }));
    await waitFor(() => expect(pressed("Changes")).toBe("true"));

    rerender(page("o-2"));
    await openHistory();

    // Back to the whole history, which is where a reader starts on a record
    // they have not looked at yet.
    await waitFor(() => expect(pressed("All")).toBe("true"));
  });
});

describe("CompanyScreen — the active tab is scoped to the account being read", () => {
  it("opens on Overview when another company is opened", async () => {
    stubFetch(async (url) =>
      /\/companies\/o-\d$/.test(new URL(url).pathname)
        ? jsonResponse(company)
        : emptyPage(),
    );
    // One QueryClient across both renders, with both records already cached.
    // That keeps the record component mounted across the swap, as in the
    // chronology filter's test above.
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    client.setQueryData(["company", "o-1"], company);
    client.setQueryData(["company", "o-2"], { ...company, id: "o-2" });
    const page = (id: string) => (
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <CompanyScreen id={id} />
        </LocaleProvider>
      </QueryClientProvider>
    );
    const { rerender } = rtlRender(page("o-1"));
    const pressed = (name: string) =>
      screen.getByRole("button", { name }).getAttribute("aria-pressed");

    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Documents" })).toBeTruthy(),
    );
    await userEvent.click(screen.getByRole("button", { name: "Documents" }));
    await waitFor(() => expect(pressed("Documents")).toBe("true"));

    rerender(page("o-2"));

    await waitFor(() => expect(pressed("Overview")).toBe("true"));
  });
});

describe("CompanyScreen — the Partner tab is scoped to the account being read", () => {
  // A company with a programme carries "partner" in relationship_types. One
  // without it must never offer the tab, nor inherit it from the account
  // open before.
  const partnerCompany = {
    ...company,
    relationship_types: ["partner"] as const,
  };
  const nonPartnerCompany = { ...company, id: "o-2" };

  function stubTwoCompanies() {
    stubFetch(async (url) => {
      const pathname = new URL(url).pathname;
      if (pathname.endsWith("/companies/o-1")) {
        return jsonResponse(partnerCompany);
      }
      if (pathname.endsWith("/companies/o-2")) {
        return jsonResponse(nonPartnerCompany);
      }
      return emptyPage();
    });
  }

  // One QueryClient across both renders, with both records already cached.
  // That keeps the record component mounted across the swap, the only
  // condition under which a leak would show (as in the chronology filter test).
  function renderTwoCompanies() {
    const client = new QueryClient({
      defaultOptions: { queries: { retry: false } },
    });
    client.setQueryData(["company", "o-1"], partnerCompany);
    client.setQueryData(["company", "o-2"], nonPartnerCompany);
    const page = (id: string) => (
      <QueryClientProvider client={client}>
        <LocaleProvider initial="en">
          <CompanyScreen id={id} />
        </LocaleProvider>
      </QueryClientProvider>
    );
    return { ...rtlRender(page("o-1")), page };
  }

  it("does not carry the Partner tab to an account with no programme", async () => {
    stubTwoCompanies();
    const { rerender, page } = renderTwoCompanies();

    await userEvent.click(
      await screen.findByRole("button", { name: "Partner" }),
    );
    await waitFor(() =>
      expect(
        screen
          .getByRole("button", { name: "Partner" })
          .getAttribute("aria-pressed"),
      ).toBe("true"),
    );

    rerender(page("o-2"));

    await waitFor(() =>
      expect(
        screen
          .getByRole("button", { name: "Overview" })
          .getAttribute("aria-pressed"),
      ).toBe("true"),
    );
    expect(screen.queryByRole("button", { name: "Partner" })).toBeNull();
  });

  // The carveout that keeps the tab reachable (companyTabsFor's
  // `tab === "partner"` branch) must hold too. Removing it would also stop the
  // leak, and remove the reader's only way to a first partner row.
  it("still opens the set-up-partner form on an account with no programme", async () => {
    stubFetch(companyBackstop);
    render(<CompanyScreen id="o-1" />);

    await userEvent.click(
      await screen.findByRole("button", { name: "More actions" }),
    );
    await userEvent.click(
      await screen.findByRole("button", {
        name: "Set up partner program",
      }),
    );

    await waitFor(() =>
      expect(
        screen
          .getByRole("button", { name: "Partner" })
          .getAttribute("aria-pressed"),
      ).toBe("true"),
    );
  });
});
