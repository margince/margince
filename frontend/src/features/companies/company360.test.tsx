/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { meFixture } from "../../testing/mefixture";
import { NextSteps } from "./company360";
import {
  type Company360,
  company,
  emptyPage,
  jsonResponse,
  partnerCompany,
  render,
  renderCompany,
  stub,
  view,
} from "./company360.testkit";
import { sinceLastVisitFooter } from "./companywork";

// useMe only asks /v1/me once a workspace slug is resolved, and useCan denies
// until it answers — so without the slug every in-place editor on this page
// renders read-only and a test cannot tell a withheld control from a missing
// one.
beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
});

// The brief and the open-task list are components of their own, mounted here
// directly rather than through the company page: the page does not render
// either, so reaching for them through it would assert nothing.
function renderNextSteps(
  three60: Company360,
  onOpenTask?: (step: { activity_id: string }) => void,
) {
  render(<NextSteps view={three60} onOpenTask={onOpenTask} />);
}

// The since-last-visit footer, rendered on its own: the account's own reading
// carries it now, and this is the sentence, not the reading it once sat on.
function renderWork(three60: Company360) {
  render(sinceLastVisitFooter(three60) ?? null);
}

describe("company view — withheld sections", () => {
  it("says a section is hidden rather than drawing it empty", async () => {
    // Deals moved to their own tab: the card is no longer inside the
    // Business grid, so the reader has to be on the Deals tab to see it.
    stub(view({ deals: undefined, sections_omitted: ["deals"] }));
    renderCompany();

    await userEvent.click(
      await screen.findByRole("button", { name: /^Deals/ }),
    );
    const heading = await screen.findByRole("heading", { name: "Deals" });
    const deals = heading.closest("section");
    if (!deals) {
      throw new Error("the deals card has no section wrapper");
    }
    expect(within(deals).getByText("Hidden for your role")).toBeTruthy();
    // The empty state and the withheld state must never both appear: one
    // says there is nothing, the other says you may not know.
    expect(
      within(deals).queryByText("No open deals for this company."),
    ).toBeNull();
  });

  it("draws the empty state when the section is present and empty", async () => {
    stub(view());
    renderCompany();

    await userEvent.click(
      await screen.findByRole("button", { name: /^Deals/ }),
    );
    const heading = await screen.findByRole("heading", { name: "Deals" });
    const deals = heading.closest("section");
    if (!deals) {
      throw new Error("the deals card has no section wrapper");
    }
    expect(
      within(deals).getByText("No open deals for this company."),
    ).toBeTruthy();
    expect(within(deals).queryByText("Hidden for your role")).toBeNull();
  });

  it("reports no committee gap when the contacts section was withheld", async () => {
    // The gap is computed from the contact list, and a withheld section
    // arrives as the same empty array an account with no contacts does.
    // Reading "nobody here is your champion" off contacts the caller was
    // never allowed to see states a fact about data the page does not have.
    stub(
      view({
        contacts: undefined,
        sections_omitted: ["contacts"],
        deals: {
          data: [
            {
              deal_id: "d-1",
              name: "Pilot",
              status: "open",
              stalled: false,
            },
          ],
          page: emptyPage,
          won_lifetime: { amount_minor: 0, currency: "EUR" },
          lost_count: 0,
          won_count: 0,
        },
      }),
    );
    renderCompany();

    await screen.findByRole("complementary", { name: "Context" });
    expect(screen.queryByText(/Nobody here is your/)).toBeNull();
  });

  it("says the KPI row is hidden rather than dropping it silently", async () => {
    // The strip used to return null on ANY absent state_strip, which read the
    // same on a caller with no grant on it as on a page still loading. A
    // reader whose role withholds the row is told so instead of the row
    // simply never appearing.
    stub(view({ sections_omitted: ["state_strip"] }));
    renderCompany();

    const strip = await screen.findByRole("region", {
      name: "Company status",
    });
    expect(within(strip).getByText("Hidden for your role")).toBeTruthy();
  });

  it("says the health verdict is hidden rather than drawing no card", async () => {
    // Health drew nothing on a withheld section, on an account with nothing
    // rated yet, and on one with nothing to report alike. The first two are
    // facts about the READ; only the third is a fact about the account, and a
    // reader who meets no card at all takes it for the third.
    stub(
      view({
        health: undefined,
        sections_omitted: ["health"],
        state_strip: {
          account: { lifecycle: "prospect", relationship_types: [] },
        },
      }),
    );
    renderCompany();

    // The standing is the 360's word now, under the readings row: withheld
    // reads as withheld there, on the word and on each dimension.
    const call = await screen.findByText(/How it stands/);
    const pane = call.closest(".panel");
    if (!(pane instanceof HTMLElement)) {
      throw new Error("the 360 has no pane");
    }
    expect(within(pane).getAllByText("Not shown").length).toBeGreaterThan(0);
    expect(within(pane).queryByText("Not assessed")).toBeNull();
  });

  it("says there is no health reading yet, distinct from hidden", async () => {
    // `health` came back as an object with nothing rated — e.g. an account
    // that has never exchanged mail — which must read as "there is none",
    // never as the withheld notice above.
    stub(
      view({
        health: {},
        state_strip: {
          account: { lifecycle: "prospect", relationship_types: [] },
        },
      }),
    );
    renderCompany();

    const call = await screen.findByText(/How it stands/);
    const pane = call.closest(".panel");
    if (!(pane instanceof HTMLElement)) {
      throw new Error("the 360 has no pane");
    }
    expect(within(pane).getAllByText("Not assessed").length).toBeGreaterThan(0);
    expect(within(pane).queryByText("Not shown")).toBeNull();
  });

  it("rates the relationship reading once there is a signal to read", async () => {
    stub(
      view({
        health: { days_since_last_inbound: 3, active_contacts: 2 },
        state_strip: {
          account: { lifecycle: "prospect", relationship_types: [] },
        },
      }),
    );
    renderCompany();

    const strip = await screen.findByRole("region", {
      name: "Company status",
    });
    const relationship = within(strip)
      .getAllByText("Relationship")[0]
      ?.closest(".stat-card");
    if (!(relationship instanceof HTMLElement)) {
      throw new Error("the relationship card has no wrapper");
    }
    expect(within(relationship).getByText("Active")).toBeTruthy();
    expect(within(relationship).queryByText("No exchange")).toBeNull();
  });
});

describe("company view — the verbs that change a section", () => {
  it("offers New deal on an account with no open deal", async () => {
    // The empty state is exactly where a create verb belongs: a rep who has
    // just read "no open deal on this account" is one click from opening one.
    stub(view());
    renderCompany();

    await userEvent.click(
      await screen.findByRole("button", { name: /^Deals/ }),
    );
    const heading = await screen.findByRole("heading", { name: "Deals" });
    const deals = heading.closest("section");
    if (!deals) {
      throw new Error("the deals card has no section wrapper");
    }
    expect(
      within(deals).getByText("No open deals for this company."),
    ).toBeTruthy();
    // Awaited: the verb appears once the pipeline read resolves, because a
    // deal needs somewhere to land before the page offers to open one. Scoped
    // to the tab body: the rail's own Deals panel is on the same empty
    // account, so it offers the same verb and a page-wide query would be
    // ambiguous between the two.
    expect(
      await within(deals).findByRole("button", { name: "New deal" }),
    ).toBeTruthy();
  });

  it("offers no New deal on a section the caller may not read", async () => {
    // A caller who cannot read the deals has no business being offered a
    // button to add one, and the refusal must not be the first they hear of it.
    const fetched = stub(
      view({ deals: undefined, sections_omitted: ["deals"] }),
    );
    renderCompany();

    // The Deals tab body is the one surface on that tab that reads the grant.
    // The count is pinned at one, not "at least one". A floor would pass on a
    // tab that rendered nothing while some other surface still spoke.
    await userEvent.click(
      await screen.findByRole("button", { name: /^Deals/ }),
    );
    await waitFor(() =>
      expect(screen.queryAllByText("Hidden for your role")).toHaveLength(1),
    );
    // And the empty state it did NOT draw: "no open deal" is a claim about
    // the account that this payload cannot support.
    expect(screen.queryByText("No open deals for this company.")).toBeNull();
    // The absent button alone would prove nothing: the verb also renders null
    // while its pipeline read is in flight, so the assertion could pass on
    // that transient state with the guard deleted. What pins the guard is
    // that the verb never MOUNTED — it is the only thing on this page that
    // reads /pipelines, so an unfetched /pipelines means the withheld section
    // never rendered it.
    await waitFor(() =>
      expect(fetched.some((path) => path.endsWith("/360"))).toBe(true),
    );
    expect(fetched.some((path) => path.endsWith("/pipelines"))).toBe(false);
    expect(screen.queryByRole("button", { name: "New deal" })).toBeNull();
  });
});

describe("company view — the context column belongs to the account, not to a tab", () => {
  it("keeps the relationship rail mounted when the reader switches tab", async () => {
    stub(view(), 200, partnerCompany);
    renderCompany();

    await screen.findByRole("complementary", { name: "Context" });

    await userEvent.click(screen.getByRole("button", { name: "Partner" }));

    // Partner and History used to render in a header-only frame, so the side
    // columns unmounted and every query behind them refetched on the way
    // back. The LEFT rail — the account's relationship context — is passed
    // once to RecordView and rides on the page's own mount, so it never
    // unmounts across a tab switch.
    expect(screen.getByRole("complementary", { name: "Context" })).toBeTruthy();
  });

  it("does not refetch the account when the reader switches tab and back", async () => {
    const fetched = stub(view(), 200, partnerCompany);
    renderCompany();
    await screen.findByRole("complementary", { name: "Context" });
    const before = fetched.filter((path) => path.endsWith("/360")).length;

    await userEvent.click(screen.getByRole("button", { name: "Partner" }));
    await userEvent.click(screen.getByRole("button", { name: "Overview" }));

    expect(fetched.filter((path) => path.endsWith("/360")).length).toBe(before);
  });

  it("leaves the timeline to its own tab rather than repeating it under a form", async () => {
    stub(view(), 200, partnerCompany);
    renderCompany();
    await screen.findByRole("complementary", { name: "Context" });

    // The chronology moved off the overview when the page gained its own
    // History tab, so it is not under the partner form either.
    await userEvent.click(screen.getByRole("button", { name: "History" }));
    expect(screen.getByRole("region", { name: "History" })).toBeTruthy();

    await userEvent.click(screen.getByRole("button", { name: "Partner" }));
    expect(screen.queryByRole("region", { name: "History" })).toBeNull();
  });
});

describe("company view — what changed since the last visit", () => {
  it("counts only the dimensions it was allowed to count", () => {
    const three60 = view({
      since_last_visit: {
        baseline_at: "2026-05-30T09:00:00Z",
        new_activities: 3,
        // Null, not zero: the caller has no deal grant, so this dimension
        // was not counted at all and must not read as "nothing moved".
        deal_stage_moves: null,
        pending_proposals: 2,
      },
    });
    renderWork(three60);

    expect(screen.getByText("3 new items since your last visit.")).toBeTruthy();
    // The decision count has ONE display, the header chip, which counts the
    // approvals section. This block used to render its own count off
    // since_last_visit.pending_proposals, and the two disagreed on screen.
    // deal_stage_moves came back null: not counted, so the brief says nothing
    // about it rather than reporting that no deal moved.
    expect(screen.queryByText(/moved stage/)).toBeNull();
  });

  it("greets a first visit as a first visit, not as nothing having happened", () => {
    const three60 = view({
      since_last_visit: {
        baseline_at: null,
        new_activities: 0,
        deal_stage_moves: 0,
        pending_proposals: 0,
      },
    });
    renderWork(three60);

    expect(
      screen.getByText("You have not opened this company before."),
    ).toBeTruthy();
    expect(screen.queryByText("Nothing new since your last visit.")).toBeNull();
  });
});

describe("company view — next steps", () => {
  it("marks an overdue task and names what it is linked to", async () => {
    const three60 = view({
      next_steps: {
        data: [
          {
            activity_id: "a-1",
            subject: "Send the renewal paperwork",
            due_at: "2026-05-01T09:00:00Z",
            overdue: true,
            linked_deal_id: null,
            linked_contact_id: null,
            assignee_id: null,
          },
        ],
        page: emptyPage,
      },
    });
    stub(three60);
    renderNextSteps(three60);

    await waitFor(() =>
      expect(screen.getByText("Send the renewal paperwork")).toBeTruthy(),
    );
    expect(screen.getByText("Overdue")).toBeTruthy();
  });
});

describe("company view — a section still loading is not one that failed", () => {
  // The composite read hands every section the SAME undefined `view` while
  // it is still in flight and once it has actually failed — sectionState's
  // whole job is telling those two apart, and this pins the distinction it
  // is for: a read that has not answered yet draws a skeleton, and only a
  // read that answered with an error draws "could not be loaded".
  it("draws a skeleton while the composite read is still in flight, not a failure", async () => {
    let resolve360: (() => void) | undefined;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        const pathname = new URL(request.url).pathname;
        if (pathname.endsWith("/360")) {
          // Hangs until the assertions below have looked at the still-loading
          // page, then resolves so cleanup does not leave a dangling fetch.
          await new Promise<void>((resolve) => {
            resolve360 = resolve;
          });
          return jsonResponse(view());
        }
        if (pathname.endsWith("/v1/me")) {
          return jsonResponse(meFixture({ allow: { company: ["read"] } }));
        }
        if (pathname.endsWith("/companies/o-1")) {
          return jsonResponse(company);
        }
        return jsonResponse({ data: [], page: emptyPage });
      }),
    );
    renderCompany();

    await screen.findByText("Brandt Automotive GmbH");
    // The day's brief is its own card, named by its own subhead, so that is
    // the wrapper the skeleton and the settled answer both belong to. The
    // call card above it draws nothing while the read is pending: a card
    // stating a verdict has none to state yet.
    // Found again after every settle rather than held: the pending read draws
    // one card and the settled one draws the call above it, so a node captured
    // while the skeleton was up is a different card by the time the answer
    // arrives.
    const brief = () => {
      const panel = screen
        .getByRole("heading", { name: "Next" })
        .closest("section");
      if (!panel) {
        throw new Error("the day's brief has no section wrapper");
      }
      return panel;
    };
    await waitFor(() =>
      expect(brief().querySelector(".skeleton")).toBeTruthy(),
    );
    expect(within(brief()).queryByText(/Could not be loaded/)).toBeNull();

    resolve360?.();
    await waitFor(() => expect(brief().querySelector(".skeleton")).toBeNull());
    // The settled read on an account with nothing needing a contact today —
    // the honest answer the brief gives once it has actually read the account,
    // never the failure text a still-loading read would be mistaken for.
    expect(
      within(brief()).getByText("Nothing needs attention right now."),
    ).toBeTruthy();
    expect(within(brief()).queryByText(/Could not be loaded/)).toBeNull();
  });
});

describe("company view — a failed read is not an empty account", () => {
  it("says the page is partial instead of drawing a bare account", async () => {
    stub({ title: "Internal", detail: "boom" }, 500);
    renderCompany();

    await waitFor(() =>
      expect(screen.getByText(/this page may be incomplete/)).toBeTruthy(),
    );
    // The overview's Deals and Contacts sections STAY, each saying it could
    // not be loaded. Dropping them would read as an account with neither.
    for (const title of ["Deals", "Contacts"]) {
      const panel = screen
        .getAllByRole("heading", { name: title })
        .map((heading) => heading.closest<HTMLElement>(".panel"))
        .find((each) => each !== null);
      if (!panel) {
        throw new Error(`the ${title} section is not drawn`);
      }
      expect(within(panel).getByText(/did not load/)).toBeTruthy();
    }
    expect(screen.queryByText("No deals for this company yet.")).toBeNull();
    expect(screen.queryByText("No contacts yet.")).toBeNull();
  });

  it("distinguishes a section that is missing from one that is empty", async () => {
    // No `deals` key at all and nothing named in sections_omitted: the
    // server did not say the caller may not read it, and did not send it —
    // so the page knows nothing, and must not claim there are none.
    stub(view({ deals: undefined }));
    renderCompany();

    await userEvent.click(
      await screen.findByRole("button", { name: /^Deals/ }),
    );
    const heading = await screen.findByRole("heading", { name: "Deals" });
    const deals = heading.closest("section");
    if (!deals) {
      throw new Error("the deals card has no section wrapper");
    }
    expect(within(deals).getByText(/did not load/)).toBeTruthy();
    expect(
      within(deals).queryByText("No open deals for this company."),
    ).toBeNull();
    expect(within(deals).queryByText("Hidden for your role")).toBeNull();
  });
});

describe("company view — figures that outlive the list they sit under", () => {
  it("keeps the lifetime won total on an account with no open deal", async () => {
    stub(
      view({
        deals: {
          data: [],
          page: emptyPage,
          won_lifetime: { amount_minor: 12_000_000, currency: "EUR" },
          lost_count: 3,
          won_count: 0,
        },
      }),
    );
    renderCompany();

    await userEvent.click(
      await screen.findByRole("button", { name: /^Deals/ }),
    );
    // No OPEN deal is true and is said. The account still won €120,000 —
    // hiding that because today's pipeline is empty loses a real fact.
    expect(
      await screen.findByText("No open deals for this company."),
    ).toBeTruthy();
    expect(screen.getByText(/120,000/)).toBeTruthy();
    expect(screen.getByText("3 lost")).toBeTruthy();
  });
});
