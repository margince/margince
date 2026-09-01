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
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { meFixture } from "../app/mefixture";
import { RecordShell } from "../app/testing/recordshell.testkit";
import { LocaleProvider } from "../i18n";
import { taskWriteKeys } from "./activitykeys";
import {
  CommercialPanel,
  NextSteps,
  type SuggestionAction,
  SuggestionsSection,
} from "./company360";
import { CompanyWorkCard } from "./companywork";
import { CompanyScreen } from "./organizations";
import { SentenceList } from "./record360";
import { TaskQuickActions, useTaskUpdate } from "./taskactions";

// The company view's honesty rules, which are the whole point of the
// composite read:
//
//   - a section the caller's role withheld says so, and never draws the
//     empty state that would read as "there is none";
//   - consent is per purpose and default-deny, so silence never renders as
//     permission;
//   - a workspace reading from an incumbent mirror gets one refusal, not a
//     page that quietly omits most of itself.

type Organization = components["schemas"]["Organization"];
type Organization360 = components["schemas"]["Organization360"];
// The sections these tests build fixtures for, named once. A fixture declared
// standalone gets no contextual type from `view()`, so its enums widen to
// `string` and stop being checked at all — which is how eight of the shapes in
// this file drifted off the contract while every test went on passing.
type StateStrip360 = NonNullable<Organization360["state_strip"]>;

// Typed against the contract, not asserted into it. A fixture the compiler only
// sees as `Record<string, unknown>` can name a field the wire dropped, spell an
// enum the server no longer accepts, or miss one it now requires, and the tests
// built on it go on passing while the shape moves underneath them. That is the
// one thing a fixture must not do — so `view()` returns `Organization360`, and
// every caller takes it as that type rather than through `as never`.
const org: Organization = {
  id: "o-1",
  // The server answers this per row; a fixture without it reads as NOT
  // writable, which is the correct fail-closed default and would strip the
  // edit affordances these tests are about.
  writable: true,
  display_name: "Brandt Automotive GmbH",
  industry: "Automotive",
  captured_by: "human:u1",
  source: "manual",
  version: 1,
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-06-01T08:00:00Z",
};

const emptyPage = { has_more: false, next_cursor: null };

function view(overrides: Partial<Organization360> = {}): Organization360 {
  return {
    as_of: "2026-06-01T09:00:00Z",
    organization: org,
    sections_omitted: [],
    people: { data: [], page: emptyPage },
    deals: {
      data: [],
      page: emptyPage,
      won_lifetime: { amount_minor: 0, currency: "EUR" },
      lost_count: 0,
    },
    activities: { data: [], page: emptyPage },
    next_steps: { data: [], page: emptyPage },
    pending_approvals: { data: [], page: emptyPage },
    tags: [],
    list_memberships: [],
    since_last_visit: {
      baseline_at: "2026-05-30T09:00:00Z",
      new_activities: 0,
      deal_stage_moves: 0,
      pending_proposals: 0,
    },
    ...overrides,
  };
}

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "content-type": "application/json" },
  });
}

const emptyRollup = {
  root_id: "o-1",
  scope: "tree",
  weighted_pipeline: { amount_minor: 0, currency: "EUR" },
  closed_won: { amount_minor: 0, currency: "EUR" },
  activity_count_30d: 0,
  aggregated_account_count: 1,
  restricted_excluded: [],
  computed_at: "2026-06-01T09:00:00Z",
};

const EMPTY_BRIEF = {
  organization_id: "o-1",
  generated_at: "2026-06-01T09:00:00Z",
  generated_by: "deterministic",
  sentences: [],
};

// Reset after every test (see afterEach): a brief one case set for itself is
// otherwise still being served to the next one.
let briefBody: unknown = EMPTY_BRIEF;

// partnerOrg is the account that HAS a partner programme, and so carries the
// second tab. Partnerhood is read off the relationship type rather than the
// extension row, because the Organization read never selects that row — the
// two are equivalent, since the store enforces the invariant both ways.
// The bare fixture deliberately carries neither: a Partner tab on an account
// with no programme is the thing the tab gate removed.
const partnerOrg = { ...org, relationship_types: ["partner"] };

function stub(
  three60: unknown,
  status = 200,
  account: unknown = org,
  finance: unknown = { organization_id: "o-1", state: "no_connection" },
  financeStatus = 200,
) {
  // The paths actually requested. A test proves the page did NOT refetch by
  // counting these rather than by trusting that it did not.
  const fetched: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (request: Request) => {
      const pathname = new URL(request.url).pathname;
      fetched.push(pathname);
      if (pathname.endsWith("/360")) {
        return jsonResponse(three60, status);
      }
      // The People tab's list, served from the SAME people section the 360
      // fixture carries: a test that seeds a contact sees it on the tab
      // without seeding it twice in two shapes that could disagree.
      if (pathname.endsWith("/contacts")) {
        const people =
          (three60 as { people?: { data?: Record<string, unknown>[] } })?.people
            ?.data ?? [];
        return jsonResponse({
          data: people.map((contact) => ({
            person_id: contact.person_id,
            full_name: contact.full_name,
            title: contact.title,
            engagement: "untried",
            strength: contact.strength,
          })),
          page: { has_more: false, next_cursor: null },
        });
      }
      if (pathname.endsWith("/finance-summary")) {
        return jsonResponse(finance, financeStatus);
      }
      if (pathname.endsWith("/hierarchy-rollup")) {
        return jsonResponse(emptyRollup);
      }
      if (pathname.endsWith("/brief")) {
        return jsonResponse(briefBody);
      }
      if (pathname.endsWith("/graph")) {
        return jsonResponse({
          nodes: [
            { id: "u-2", kind: "user", label: "Mira", root: false },
            { id: "p-1", kind: "person", label: "Dana Buyer", root: false },
          ],
          edges: [
            {
              from: "u-2",
              to: "p-1",
              kind: "in_contact_with",
              strength: 90,
              strength_bucket: "strong",
            },
          ],
          groups_omitted: [],
          dropped_count: 0,
        });
      }
      // The viewer's grants. Without this useCan denies — it fails closed on a
      // missing snapshot — and every in-place editor on the page renders as
      // read-only text, so a test could not tell "correctly withheld" from
      // "never built".
      if (pathname.endsWith("/v1/me")) {
        return jsonResponse(
          meFixture({ allow: { organization: ["read", "update"] } }),
        );
      }
      if (pathname.endsWith("/organizations/o-1")) {
        return jsonResponse(account);
      }
      if (pathname.endsWith("/pipelines")) {
        // One default pipeline with one OPEN stage — enough for a deal
        // created from this page to have somewhere to land.
        return jsonResponse({
          data: [
            {
              id: "pl-1",
              name: "Sales",
              is_default: true,
              stages: [
                {
                  id: "st-1",
                  pipeline_id: "pl-1",
                  name: "Qualify",
                  position: 1,
                  semantic: "open",
                  probability: 10,
                },
              ],
            },
          ],
          page: emptyPage,
        });
      }
      return jsonResponse({ data: [], page: emptyPage });
    }),
  );
  return fetched;
}

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
  briefBody = EMPTY_BRIEF;
});

// The page's context column is the SHELL's, not the record's: the record fills
// it through a portal. So this render carries the real region rather than a
// stand-in — a test that supplied its own column would prove nothing about the
// one the product draws, and the record's context cards would have nowhere to
// land.
function render(ui: ReactNode) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  return rtlRender(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <RecordShell>{ui}</RecordShell>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

function renderCompany() {
  render(<CompanyScreen id="o-1" />);
}

// The brief and the open-task list are components of their own, mounted here
// directly rather than through the company page: the page does not render
// either, so reaching for them through it would assert nothing.
function renderNextSteps(
  three60: Organization360,
  onOpenTask?: (step: { activity_id: string }) => void,
) {
  render(<NextSteps view={three60} onOpenTask={onOpenTask} />);
}

// NextSteps plus the per-row verbs, which the list takes as a render slot
// rather than owning. Mounting the two together is what pins the pairing the
// suite is about: a row offers Done always and Snooze only when there is a
// date to move.
function NextStepsWithVerbs({
  three60,
}: Readonly<{ three60: Organization360 }>) {
  const update = useTaskUpdate(taskWriteKeys("organization", "o-1"));
  return (
    <NextSteps
      view={three60}
      onOpenTask={() => {}}
      renderAction={(step) => (
        <TaskQuickActions
          activityId={step.activity_id}
          dueAt={step.due_at}
          update={update}
        />
      )}
    />
  );
}

function renderWork(three60: Organization360) {
  render(<CompanyWorkCard view={three60} onOpenRecord={() => {}} />);
}

// The lead panel, rendered on its own: it moved out of AccountBrief so the
// stack could tint and box it separately, and the advice it carries is
// exercised through it directly now.
function renderSuggestions(
  three60: Organization360,
  onPerform: (action: SuggestionAction) => void = () => {},
) {
  render(
    <SuggestionsSection
      orgId="o-1"
      view={three60}
      onOpenRecord={() => {}}
      onPerform={onPerform}
    />,
  );
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
    expect(
      within(deals).getByText("Hidden — your role cannot read this"),
    ).toBeTruthy();
    // The empty state and the withheld state must never both appear: one
    // says there is nothing, the other says you may not know.
    expect(
      within(deals).queryByText("No open deal on this account."),
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
      within(deals).getByText("No open deal on this account."),
    ).toBeTruthy();
    expect(
      within(deals).queryByText("Hidden — your role cannot read this"),
    ).toBeNull();
  });

  it("reports no committee gap when the people section was withheld", async () => {
    // The gap is computed from the contact list, and a withheld section
    // arrives as the same empty array an account with no contacts does.
    // Reading "nobody here is your champion" off contacts the caller was
    // never allowed to see states a fact about data the page does not have.
    stub(
      view({
        people: undefined,
        sections_omitted: ["people"],
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
      name: "Where this account stands",
    });
    expect(
      within(strip).getByText("Hidden — your role cannot read this"),
    ).toBeTruthy();
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

    const strip = await screen.findByRole("region", {
      name: "Where this account stands",
    });
    const health = within(strip).getByText("Health").closest(".stat-card");
    if (!(health instanceof HTMLElement)) {
      throw new Error("the health verdict card has no wrapper");
    }
    expect(within(health).getByText("Not shown")).toBeTruthy();
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

    const strip = await screen.findByRole("region", {
      name: "Where this account stands",
    });
    const health = within(strip).getByText("Health").closest(".stat-card");
    if (!(health instanceof HTMLElement)) {
      throw new Error("the health verdict card has no wrapper");
    }
    expect(within(health).getByText("Not assessed")).toBeTruthy();
    expect(within(health).getByText("0 of 3 rated")).toBeTruthy();
    expect(within(health).queryByText("Not shown")).toBeNull();
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
      name: "Where this account stands",
    });
    const relationship = within(strip)
      .getByText("Conversation")
      .closest(".stat-card");
    if (!(relationship instanceof HTMLElement)) {
      throw new Error("the relationship card has no wrapper");
    }
    expect(within(relationship).getByText("In conversation")).toBeTruthy();
    expect(
      within(relationship).queryByText("They have never written"),
    ).toBeNull();
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
      within(deals).getByText("No open deal on this account."),
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

    // Four surfaces read the same missing grant: the header's own fact strip
    // (Open pipeline, In flight), the rail's Deals panel — which rides on the
    // page's own mount and never unmounts across a tab switch — and the Deals
    // tab body itself. The count is pinned EXACTLY rather than as "at least
    // one": a floor would pass on a tab that rendered nothing while some other
    // surface still spoke.
    await userEvent.click(
      await screen.findByRole("button", { name: /^Deals/ }),
    );
    await waitFor(() =>
      expect(
        screen.queryAllByText("Hidden — your role cannot read this"),
      ).toHaveLength(4),
    );
    // And the empty state it did NOT draw: "no open deal" is a claim about
    // the account that this payload cannot support.
    expect(screen.queryByText("No open deal on this account.")).toBeNull();
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

it("offers the tag verb but not the list verb when only lists are withheld", async () => {
  // The two halves of the card are governed separately, so one withheld
  // grant must not take the other's verb with it — and must not offer a
  // write whose refusal would be the first the reader hears of the limit.
  stub(
    view({
      list_memberships: undefined,
      sections_omitted: ["list_memberships"],
    }),
  );
  renderCompany();

  // The verbs sit in the overview stack now — the rail (companyrail.tsx)
  // shows the tags and lists themselves, and carries no write affordance of
  // its own, so the action strip that changes them is unscoped here.
  expect(await screen.findByRole("button", { name: "Add tag" })).toBeTruthy();
  expect(screen.queryByRole("button", { name: "Add to list" })).toBeNull();
});

describe("company view — the context column belongs to the account, not to a tab", () => {
  it("keeps the relationship rail mounted when the reader switches tab", async () => {
    stub(view(), 200, partnerOrg);
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
    const fetched = stub(view(), 200, partnerOrg);
    renderCompany();
    await screen.findByRole("complementary", { name: "Context" });
    const before = fetched.filter((path) => path.endsWith("/360")).length;

    await userEvent.click(screen.getByRole("button", { name: "Partner" }));
    await userEvent.click(screen.getByRole("button", { name: "Overview" }));

    expect(fetched.filter((path) => path.endsWith("/360")).length).toBe(before);
  });

  it("leaves the timeline to its own tab rather than repeating it under a form", async () => {
    stub(view(), 200, partnerOrg);
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

describe("company view — overlay mode", () => {
  it("refuses once instead of rendering a page missing most of itself", async () => {
    stub(
      {
        title: "Unprocessable",
        code: "validation_error",
        details: {
          errors: [
            { field: "id", code: "unsupported_in_overlay_mode", message: "x" },
          ],
        },
      },
      422,
    );
    renderCompany();

    await waitFor(() =>
      expect(screen.getByText(/not assembled here/)).toBeTruthy(),
    );
    // No half-page: the overview's own panels (the account, its worth, the
    // pipeline, the money) are absent entirely rather than showing cards
    // that would each read as an empty account.
    expect(document.querySelector(".co-panel-stack")?.textContent).toBeFalsy();
  });
});

describe("company view — what changed since the last visit", () => {
  it("counts only the dimensions it was allowed to count", async () => {
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
    stub(three60);
    renderWork(three60);

    await waitFor(() =>
      expect(
        screen.getByText("3 new items since your last visit."),
      ).toBeTruthy(),
    );
    // The decision count has ONE display, the header chip, which counts the
    // approvals section. This block used to render its own count off
    // since_last_visit.pending_proposals, and the two disagreed on screen.
    // deal_stage_moves came back null: not counted, so the brief says nothing
    // about it rather than reporting that no deal moved.
    expect(screen.queryByText(/moved stage/)).toBeNull();
  });

  it("greets a first visit as a first visit, not as nothing having happened", async () => {
    const three60 = view({
      since_last_visit: {
        baseline_at: null,
        new_activities: 0,
        deal_stage_moves: 0,
        pending_proposals: 0,
      },
    });
    stub(three60);
    renderWork(three60);

    await waitFor(() =>
      expect(
        screen.getByText("You are opening this account for the first time."),
      ).toBeTruthy(),
    );
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
            linked_person_id: null,
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
          return jsonResponse(meFixture({ allow: { organization: ["read"] } }));
        }
        if (pathname.endsWith("/organizations/o-1")) {
          return jsonResponse(org);
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
        .getByRole("heading", { name: "What needs a person today" })
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
    // The settled read on an account with nothing needing a person today —
    // the honest answer the brief gives once it has actually read the account,
    // never the failure text a still-loading read would be mistaken for.
    expect(
      within(brief()).getByText("Nothing here needs you today."),
    ).toBeTruthy();
    expect(within(brief()).queryByText(/Could not be loaded/)).toBeNull();
  });
});

describe("company view — a failed read is not an empty account", () => {
  it("says the page is partial instead of drawing a bare account", async () => {
    stub({ title: "Internal", detail: "boom" }, 500);
    renderCompany();

    await waitFor(() =>
      expect(screen.getByText(/may not show everything/)).toBeTruthy(),
    );
    // The business rail STAYS, with each card saying it could not be loaded.
    // Removing it would read as an account with no people and no deals,
    // which is the one thing this page does not know.
    const card = screen.getByRole("complementary", { name: "Context" });
    expect(
      within(card).getAllByText(/Could not be loaded/).length,
    ).toBeGreaterThan(0);
    expect(
      within(card).queryByText("No open deal on this account."),
    ).toBeNull();
    expect(
      within(card).queryByText("No contact linked to this account yet."),
    ).toBeNull();
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
    expect(within(deals).getByText(/Could not be loaded/)).toBeTruthy();
    expect(
      within(deals).queryByText("No open deal on this account."),
    ).toBeNull();
    expect(
      within(deals).queryByText("Hidden — your role cannot read this"),
    ).toBeNull();
  });
});

describe("company view — one section never answers for another", () => {
  it("does not let readable tags claim there are no lists", async () => {
    // Tags came back empty; lists were withheld. "Not on any list, and no
    // tags applied" would be a claim about a half nobody answered for.
    stub(
      view({
        tags: [],
        list_memberships: undefined,
        sections_omitted: ["list_memberships"],
      }),
    );
    renderCompany();

    const rail = await screen.findByRole("complementary", { name: "Context" });
    // The refusal has to name WHICH half it is about: under a heading
    // covering both, an unattached "hidden from you" leaves the reader
    // unable to tell whether the lists or the tags were withheld.
    const listsPart = within(rail).getByRole("region", { name: "Lists" });
    expect(
      within(listsPart).getByText("Hidden — your role cannot read this"),
    ).toBeTruthy();

    const tagsPart = within(rail).getByRole("region", { name: "Tags" });
    expect(within(tagsPart).getByText("No tags applied.")).toBeTruthy();
    expect(
      within(tagsPart).queryByText("Hidden — your role cannot read this"),
    ).toBeNull();
    expect(within(rail).queryByText("Not on any list.")).toBeNull();
  });

  it("still shows the tags a caller can read when lists are withheld", async () => {
    stub(
      view({
        tags: [{ id: "t-1", name: "Key account" }],
        list_memberships: undefined,
        sections_omitted: ["list_memberships"],
      }),
    );
    renderCompany();

    const rail = await screen.findByRole("complementary", { name: "Context" });
    // Losing one grant narrows the card; it does not blank it.
    await waitFor(() =>
      expect(within(rail).getByText("Key account")).toBeTruthy(),
    );
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
      await screen.findByText("No open deal on this account."),
    ).toBeTruthy();
    expect(screen.getByText(/120,000/)).toBeTruthy();
    expect(screen.getByText("3 lost")).toBeTruthy();
  });
});

// The company page's own affordances: what it says is waiting, and what a
// reader can do about it without leaving the account.

describe("company view — the citations under a finding", () => {
  // The chips are shared by every grounded surface on this page. They are
  // exercised through the advice the brief carries, which is where a reader
  // meets them now that the standing summary card is gone.
  // The citations are typed from the contract rather than taken as `unknown[]`:
  // an `entity_type` the server no longer serves is exactly the drift these
  // chips would go on rendering without complaint.
  type Suggestion = NonNullable<Organization360["suggestions"]>[number];
  const suggestion = (evidence: Suggestion["evidence"]) => ({
    kind: "no_reply" as const,
    fingerprint: "f-1",
    reason: "You reached out 13 days ago and nobody has come back.",
    evidence,
  });

  it("collapses several sources of one unopenable kind into one counted chip", async () => {
    const three60 = view({
      suggestions: [
        suggestion([
          { entity_type: "activity", entity_id: "a-1" },
          { entity_type: "activity", entity_id: "a-2" },
          { entity_type: "activity", entity_id: "a-3" },
        ]),
      ],
    });
    stub(three60);
    renderSuggestions(three60);
    // The chips live behind the reason the rule fired, where a reader who
    // doubts the advice goes to check it.
    await waitFor(() =>
      expect(
        screen.getByText(
          "You reached out 13 days ago and nobody has come back.",
        ),
      ).toBeTruthy(),
    );
    await userEvent.click(
      screen.getByText("You reached out 13 days ago and nobody has come back."),
    );
    // Not "activityactivityactivity": one chip that says how many.
    expect(screen.getByText("3 activities")).toBeTruthy();
    expect(screen.queryAllByText("activity")).toHaveLength(0);
  });

  // Josh's rule, and the page's own: a citation reads as the record's NAME
  // wherever the page holds one. The server names what its writer had at hand;
  // this account's 360 holds the rest.
  it("names the record from the account's own roster when the citation does not", async () => {
    const three60 = view({
      people: {
        data: [
          {
            person_id: "p-9",
            full_name: "Frédéric de Gombert",
            deal_roles: [],
            consent: {},
            strength: {
              score: 0,
              bucket: "none" as const,
              factors: {
                recency: 0,
                frequency: 0,
                reciprocity: 0,
                direction: 0,
              },
            },
          },
        ],
        page: { has_more: false },
      },
      suggestions: [suggestion([{ entity_type: "person", entity_id: "p-9" }])],
    });
    stub(three60);
    renderSuggestions(three60);
    await waitFor(() =>
      expect(
        screen.getByText(
          "You reached out 13 days ago and nobody has come back.",
        ),
      ).toBeTruthy(),
    );
    await userEvent.click(
      screen.getByText("You reached out 13 days ago and nobody has come back."),
    );

    expect(
      screen.getByRole("button", { name: "Frédéric de Gombert" }),
    ).toBeTruthy();
    expect(screen.queryByText("contact")).toBeNull();
  });

  it("counts one record cited twice as one source", async () => {
    const three60 = view({
      suggestions: [
        suggestion([
          { entity_type: "activity", entity_id: "a-1" },
          { entity_type: "activity", entity_id: "a-1" },
        ]),
      ],
    });
    stub(three60);
    renderSuggestions(three60);
    await waitFor(() =>
      expect(
        screen.getByText(
          "You reached out 13 days ago and nobody has come back.",
        ),
      ).toBeTruthy(),
    );
    await userEvent.click(
      screen.getByText("You reached out 13 days ago and nobody has come back."),
    );
    expect(screen.getByText("activity")).toBeTruthy();
    expect(screen.queryByText("2 activities")).toBeNull();
  });

  // The dossier's "collected" mode gathers every sentence's citations under
  // one row, which is exactly where several profile fields — a kind with no
  // name of its own beyond "profile field" — used to print as ten identical
  // buttons in a row rather than as one counted one.
  it("collapses several sources of one OPENABLE receipt kind into one counted chip", async () => {
    const opened: unknown[] = [];
    render(
      <SentenceList
        sentences={[
          {
            text: "The company operates from three regional offices.",
            evidence: [
              { entity_type: "profile_field", entity_id: "pf-1" },
              { entity_type: "profile_field", entity_id: "pf-2" },
              { entity_type: "profile_field", entity_id: "pf-3" },
            ],
          },
        ]}
        onOpenRecord={(...args) => opened.push(args)}
        citations="collected"
      />,
    );
    expect(screen.getByText("3 profile fields")).toBeTruthy();
    expect(screen.queryAllByText("profile field")).toHaveLength(0);
    // Opens the FIRST record, with every sibling so the drawer's own stepper
    // reaches the other two — the count names them, the stepper reaches them.
    await userEvent.click(screen.getByText("3 profile fields"));
    expect(opened).toEqual([
      [
        "profile_field",
        "pf-1",
        [
          { entityType: "profile_field", entityId: "pf-1" },
          { entityType: "profile_field", entityId: "pf-2" },
          { entityType: "profile_field", entityId: "pf-3" },
        ],
      ],
    ]);
  });

  // deal/person each open their OWN screen rather than a shared stepper, so
  // grouping them the same way would silently drop every record past the
  // first — they stay one chip per record instead.
  it("keeps a separate chip per record for a kind with its own screen", async () => {
    render(
      <SentenceList
        sentences={[
          {
            text: "Two deals are open with this account.",
            evidence: [
              { entity_type: "deal", entity_id: "d-1" },
              { entity_type: "deal", entity_id: "d-2" },
            ],
          },
        ]}
        onOpenRecord={() => {}}
        citations="collected"
      />,
    );
    expect(screen.getAllByText("deal")).toHaveLength(2);
    expect(screen.queryByText("2 deals")).toBeNull();
  });
});

describe("company view — what is waiting on a decision", () => {
  // `status` is a closed union on the wire, so it is annotated rather than
  // inferred as `string`: a fixture that widens it can name a status the server
  // never sends and the card would go on drawing it.
  type StagedApproval = NonNullable<
    Organization360["pending_approvals"]
  >["data"][number];
  const staged: StagedApproval = {
    id: "ap-1",
    kind: "site_lead",
    status: "pending",
    summary: "Add Markus Bueckle as a contact",
    proposed_change: { full_name: "Markus Bueckle" },
    proposed_by: "agent:capture",
    target_entity_type: "organization",
    target_entity_id: "o-1",
    diff_hash: "h1",
    created_at: "2026-06-01T08:00:00Z",
    evidence: [],
  };

  it("offers a way into the queue it counts, grouped by what is proposed", async () => {
    stub(
      view({
        pending_approvals: {
          data: [staged, { ...staged, id: "ap-2" }],
          page: emptyPage,
        },
        since_last_visit: {
          baseline_at: "2026-05-30T09:00:00Z",
          new_activities: 0,
          deal_stage_moves: 0,
          pending_proposals: 2,
        },
      }),
    );
    renderCompany();
    // The way into the queue sits with the record's other rare verbs rather
    // than beside the account's name: it opens a queue, it does not decide
    // anything, and drawn in the header it outranked the verbs that do.
    (await screen.findByRole("button", { name: "More actions" })).click();
    const open = await screen.findByRole("button", {
      name: "Review 2 waiting",
    });
    open.click();
    await waitFor(() =>
      expect(
        screen.getByText("2 × Add a person found on the site"),
      ).toBeTruthy(),
    );
  });

  it("says nothing is waiting rather than offering an empty queue", async () => {
    stub(view());
    renderCompany();
    await screen.findByText("Brandt Automotive GmbH");
    (await screen.findByRole("button", { name: "More actions" })).click();
    expect(screen.queryByRole("button", { name: /Review/ })).toBeNull();
  });
});

describe("company view — an open task can be acted on", () => {
  const step = {
    activity_id: "t-1",
    subject: "Send the retrofit proposal",
    due_at: "2026-06-10T09:00:00Z",
    overdue: false,
    linked_deal_id: null,
    linked_person_id: null,
    assignee_id: null,
  };

  it("renders the subject as a way to open the task, with the two verbs beside it", async () => {
    const three60 = view({ next_steps: { data: [step], page: emptyPage } });
    stub(three60);
    render(<NextStepsWithVerbs three60={three60} />);
    await waitFor(() =>
      expect(
        screen.getByRole("button", { name: "Send the retrofit proposal" }),
      ).toBeTruthy(),
    );
    expect(screen.getByRole("button", { name: "Done" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Snooze 1d" })).toBeTruthy();
  });

  it("offers no snooze for a task with no date to move", async () => {
    const three60 = view({
      next_steps: { data: [{ ...step, due_at: null }], page: emptyPage },
    });
    stub(three60);
    render(<NextStepsWithVerbs three60={three60} />);
    await waitFor(() =>
      expect(screen.getByRole("button", { name: "Done" })).toBeTruthy(),
    );
    expect(screen.queryByRole("button", { name: "Snooze 1d" })).toBeNull();
  });

  it("draws no checkbox when the caller has no write path for it", async () => {
    const three60 = view({ next_steps: { data: [step], page: emptyPage } });
    stub(three60);
    render(<NextSteps view={three60} />);
    await waitFor(() =>
      expect(screen.getByText("Send the retrofit proposal")).toBeTruthy(),
    );
    expect(screen.queryByRole("checkbox")).toBeNull();
  });
});

describe("CommercialPanel — a capped deals page says so", () => {
  const openDeal = {
    deal_id: "d-1",
    name: "Pilot rollout",
    status: "open" as const,
    stalled: false,
  };

  it("names the truncation rather than reading as the whole pipeline", async () => {
    const three60 = view({
      deals: {
        data: [openDeal],
        page: { has_more: true, next_cursor: "c2" },
        won_lifetime: { amount_minor: 0, currency: "EUR" },
        lost_count: 0,
      },
    });
    render(<CommercialPanel view={three60} />);
    await waitFor(() => expect(screen.getByText("Pilot rollout")).toBeTruthy());
    expect(
      screen.getByText(
        "This account has more open deals than fit here. Open All deals to see the rest.",
      ),
    ).toBeTruthy();
  });

  it("draws no truncation notice on a page that holds every open deal", async () => {
    const three60 = view({
      deals: {
        data: [openDeal],
        page: emptyPage,
        won_lifetime: { amount_minor: 0, currency: "EUR" },
        lost_count: 0,
      },
    });
    render(<CommercialPanel view={three60} />);
    await waitFor(() => expect(screen.getByText("Pilot rollout")).toBeTruthy());
    expect(
      screen.queryByText(
        "This account has more open deals than fit here. Open All deals to see the rest.",
      ),
    ).toBeNull();
  });
});

// The page said "nobody here is your champion" and gave the reader nowhere to
// say who is: the roles live on relationship rows written from the deal
// screen. The warning was true, unactionable and permanent.

// A role belongs to a deal, so the same person can be champion on one and
// nobody on another. Rendering the role alone made two clauses that read
// identically.

describe("company view — Partner is not a permanent tab", () => {
  it("offers the account's own tabs but not Partner on an account with no programme", async () => {
    stub(view());
    renderCompany();
    await screen.findByRole("complementary", { name: "Context" });

    // 360, People and History belong to every account. Partner is a form
    // about a commercial arrangement almost none of them have.
    expect(screen.getByRole("button", { name: "Overview" })).toBeTruthy();
    expect(screen.getByRole("button", { name: /^People/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: "History" })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Partner" })).toBeNull();
  });

  it("shows both tabs once the account has a programme", async () => {
    stub(view(), 200, partnerOrg);
    renderCompany();
    await screen.findByRole("complementary", { name: "Context" });

    expect(screen.getByRole("button", { name: "Partner" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Overview" })).toBeTruthy();
  });

  it("keeps the setup form reachable, so a first partner row can still be made", async () => {
    stub(view());
    renderCompany();
    await screen.findByRole("complementary", { name: "Context" });

    await userEvent.click(screen.getByRole("button", { name: "More actions" }));
    await userEvent.click(
      screen.getByRole("button", { name: "Set up partner programme" }),
    );

    // Asking for the form is what puts the tab on screen — without this the
    // hidden tab would have made the first partner row unreachable.
    expect(screen.getByRole("button", { name: "Partner" })).toBeTruthy();
  });
});

describe("company view — the visit baseline", () => {
  it("acknowledges the visit after the reader has stayed", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const fetched = stub(view());
      renderCompany();
      await screen.findByRole("complementary", { name: "Context" });
      expect(fetched.some((path) => path.endsWith("/view-ack"))).toBe(false);

      await vi.advanceTimersByTimeAsync(5_000);

      await waitFor(() =>
        expect(fetched.some((path) => path.endsWith("/view-ack"))).toBe(true),
      );
    } finally {
      vi.useRealTimers();
    }
  });

  it("does not acknowledge a visit the reader bounced straight out of", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    try {
      const fetched = stub(view());
      const { unmount } = render(<CompanyScreen id="o-1" />);
      await screen.findByRole("complementary", { name: "Context" });
      unmount();

      await vi.advanceTimersByTimeAsync(30_000);

      // Marking unread activity as seen because a record flashed past is the
      // one failure this baseline must never have.
      expect(fetched.some((path) => path.endsWith("/view-ack"))).toBe(false);
    } finally {
      vi.useRealTimers();
    }
  });
});

describe("company view — where the account stands, and what it is to us", () => {
  it("shows where the account stands, and the types it does not already say", async () => {
    // Not "partner": that type also raises the Partner tab, and the badge and
    // the tab would then share a label, which tells the test nothing.
    // The SAME override goes into both reads the page makes — the composite
    // 360's own `organization` (what the rail's grid reads) and the standalone
    // GET (what the header reads) — because now that both draw a lifecycle
    // control, a fixture that only overrode one would fail for the same
    // reason two real reads of one row never disagree: there is only one row.
    const withLifecycle: Organization = {
      ...org,
      lifecycle: "former_customer",
      relationship_types: ["customer", "supplier"],
    };
    stub(view({ organization: withLifecycle }), 200, withLifecycle);
    renderCompany();
    await screen.findByRole("complementary", { name: "Context" });

    // The retired classification held ONE value, which is how an account whose
    // contract had ended still read as "Prospect" while it was also a partner.
    // Lifecycle is now the editable control; the types stay read-only badges.
    // findAllBy, not getBy: the controls appear only once /me answers with
    // the viewer's grants, which resolves independently of the 360 awaited
    // above. TWO controls, not one — the header's pulse line and the rail's
    // Details grid both mount `CompanyLifecycleControl`, the SAME
    // implementation reused rather than a second one, so both show the same
    // value and either one writes through the same patch.
    const controls = await screen.findAllByRole("button", {
      name: "Change Account lifecycle",
    });
    expect(controls).toHaveLength(2);
    for (const control of controls) {
      expect(control.textContent).toContain("Former customer");
    }
    // A type the lifecycle already speaks for is NOT drawn beside it. This
    // account is a former customer and still carries the `customer` type,
    // because that is what it was — printed together they read as "Former
    // customer" and "Customer" on one line, which is not two facts but one
    // fact and its own contradiction.
    expect(screen.queryByText("Customer")).toBeNull();
    // Supplier survives: the lifecycle has nothing to say about it, so it is
    // a second fact rather than a second tense of the first.
    expect(screen.getByText("Supplier")).toBeTruthy();
  });

  it("offers the lifecycle control on an account nobody has assessed yet", async () => {
    stub(view(), 200, { ...org, lifecycle: "unknown", relationship_types: [] });
    renderCompany();
    await screen.findByRole("complementary", { name: "Context" });

    // 'unknown' used to draw nothing, on the reasoning that a badge announcing
    // "nobody has assessed this" is noise. That holds for a badge and breaks
    // for a control: hiding it at 'unknown' takes the field away from exactly
    // the account that needs it set, and there is no other way in from here.
    // What it must NOT do is read as a verdict, which is why it carries the
    // field name and 'Not assessed' never stands on its own. Both mount
    // points (header, grid) show it, since both draw the same control.
    const controls = await screen.findAllByRole("button", {
      name: "Change Account lifecycle",
    });
    expect(controls).toHaveLength(2);
    for (const control of controls) {
      expect(control.textContent).toContain("Not assessed");
    }
  });

  it("writes lifecycle once and shows the new value on both mount points", async () => {
    // The one thing "one implementation, two mount points" actually promises:
    // a save through EITHER control reaches the server exactly once, and the
    // OTHER control reflects the new value once the record refetches — not a
    // second write, and not one control left showing the stale value.
    let currentOrg: Organization = { ...org, lifecycle: "unknown" };
    let patchCount = 0;
    let lastIfMatch: string | null = null;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (request: Request) => {
        const pathname = new URL(request.url).pathname;
        if (pathname.endsWith("/360")) {
          return jsonResponse(view({ organization: currentOrg }));
        }
        if (pathname.endsWith("/v1/me")) {
          return jsonResponse(
            meFixture({ allow: { organization: ["read", "update"] } }),
          );
        }
        if (pathname.endsWith("/organizations/o-1")) {
          if (request.method === "PATCH") {
            patchCount += 1;
            lastIfMatch = request.headers.get("if-match");
            // Read as unknown and CHECKED, not asserted: an assertion here
            // would let a screen that sent the wrong shape — or nothing at all —
            // pass as a write of `undefined`, and the case below would then be
            // asserting against a value this stub invented.
            const sent: unknown = await request.json();
            if (sent === null || typeof sent !== "object") {
              throw new Error(`the PATCH body is not an object: ${sent}`);
            }
            const body: { lifecycle?: Organization["lifecycle"] } = sent;
            if (currentOrg.version === undefined) {
              // What this case asserts is the If-Match the second write sends,
              // so a fixture with no version to increment would be measuring
              // nothing.
              throw new Error("the account fixture carries no version");
            }
            currentOrg = {
              ...currentOrg,
              lifecycle: body.lifecycle ?? currentOrg.lifecycle,
              version: currentOrg.version + 1,
            };
          }
          return jsonResponse(currentOrg);
        }
        return jsonResponse({ data: [], page: emptyPage });
      }),
    );
    renderCompany();
    await screen.findByRole("complementary", { name: "Context" });

    const [headerControl] = await screen.findAllByRole("button", {
      name: "Change Account lifecycle",
    });
    await userEvent.click(headerControl);
    await userEvent.click(screen.getByRole("option", { name: "Prospect" }));

    await waitFor(() => expect(patchCount).toBe(1));
    expect(lastIfMatch).toBe("1");
    // Both controls now read "Prospect" — the header's own and the grid's
    // reused copy — because the single write invalidates the one query both
    // read from, not because either wrote a second time.
    await waitFor(async () => {
      const updated = await screen.findAllByRole("button", {
        name: "Change Account lifecycle",
      });
      expect(updated).toHaveLength(2);
      for (const control of updated) {
        expect(control.textContent).toContain("Prospect");
      }
    });
  });
});

// §4.2's "never render" list is the hard half of the KPI row, and each case
// below is one of its bullets. They are about what the page must NOT claim,
// which is exactly what a refactor loses silently.
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
      name: "Where this account stands",
    });

    // A zero here would claim a priced pipeline worth nothing. The truth is
    // that the page cannot price this one, so it reports the count instead.
    // No currency figure AT ALL — not merely no zero. A stray non-zero total
    // would be the worse failure, and the loose form would have passed it.
    expect(strip.textContent).not.toMatch(/[€$£]/);
    expect(within(strip).getByText("2 open")).toBeTruthy();
    expect(
      within(strip).getByText("No convertible amount on these deals"),
    ).toBeTruthy();
  });

  // Two cards saying "in conversation" in different words is one card's worth
  // of information taking two slots of six. On a live account the health
  // card reports the BALANCE of the exchange, which the engagement card does
  // not answer: they write and we do not reply, and we write into silence,
  // are both recent and are opposite problems.
  it("reports who is carrying a live relationship, not that it is live", async () => {
    stub(
      view({
        health: { days_since_last_inbound: 0, reply_balance: 0.86 },
        state_strip: {
          account: { lifecycle: "customer", relationship_types: [] },
          engagement: {
            state: "active",
            last_inbound_at: "2026-08-08T10:00:00Z",
          },
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
    const strip = await screen.findByRole("region", {
      name: "Where this account stands",
    });

    // 86% of the exchange is theirs: they are asking more than we answer.
    expect(within(strip).getByText("One-sided")).toBeTruthy();
    expect(
      within(strip).getByText(/86% of the exchange is theirs/),
    ).toBeTruthy();
  });

  it("says an empty pipeline is empty, not unpriced", async () => {
    stub(view({ state_strip: commercial({ open_count: 0 }) }));
    renderCompany();
    const strip = await screen.findByRole("region", {
      name: "Where this account stands",
    });

    // "No convertible amount" on an account with nothing open reports a data
    // problem where the truth is that nothing is running.
    expect(within(strip).getByText("No open deals")).toBeTruthy();
    expect(strip.textContent).not.toContain("No convertible amount");
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
      name: "Where this account stands",
    });

    // §4.2 bars a cross-currency sum with no conversion source and as-of date.
    // The date is the oldest rate behind the figure — how far back any part of
    // it reaches.
    // The DATE itself, not just the prefix: a dropped or wrong interpolation
    // is exactly the failure this qualification exists to prevent.
    expect(
      within(strip).getByText(/1 converted, rates from .*2026/),
    ).toBeTruthy();
  });

  it("keeps saying the pipeline is unpriced even when a deal has stalled", async () => {
    stub(view({ state_strip: commercial({ stalled_count: 1 }) }));
    renderCompany();
    const strip = await screen.findByRole("region", {
      name: "Where this account stands",
    });

    // A reader told only "1 stalled" has no way to know the pipeline carries
    // no figure at all. Both qualifications are true, so both are shown.
    expect(strip.textContent).toContain("No convertible amount on these deals");
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
      name: "Where this account stands",
    });

    // A sum covering one of two deals, shown bare, reads as the whole
    // pipeline — the unlabelled cross-currency total §4.2 forbids.
    expect(within(strip).getByText("1 of 2 deals priced")).toBeTruthy();
  });

  it("labels the sum of open deals Open pipeline, never revenue or potential", async () => {
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
      name: "Where this account stands",
    });

    expect(within(strip).getByText("Open pipeline")).toBeTruthy();
    expect(strip.textContent).not.toMatch(/revenue|potential/i);
  });

  // §4.2 gives customers and prospects different questions. A customer's page
  // is asked how it is going with them; a prospect's money card says it has
  // never been billed rather than borrowing a customer's figure. Both rows
  // still carry the account's own standing — relationship and health — which
  // is not a question the lifecycle gets to withhold.
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
      name: "Where this account stands",
    });
    expect(within(strip).getByText("Not a customer yet")).toBeTruthy();
    // Conversation and health are on BOTH rows — a prospect is not asked to
    // give up knowing how the relationship stands.
    expect(within(strip).getByText("Conversation")).toBeTruthy();

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
      name: "Where this account stands",
    });
    expect(within(strip).getByText("Conversation")).toBeTruthy();
    expect(within(strip).getByText("Gone quiet")).toBeTruthy();
    expect(within(strip).queryByText("Not a customer yet")).toBeNull();
  });
});

describe("company view — the state strip", () => {
  // Whose move it is and the worst open signal both moved to the daily
  // brief (companytoday.tsx): the strip now carries the account's STANDING
  // state only, and those two readings are DATED. Their own coverage lives
  // in companytoday.test.tsx; this suite only has to prove the strip no
  // longer draws them.
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
          // under it rather than being silently accepted by a loose stub.
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
    await screen.findByRole("region", { name: "Where this account stands" });
    const strip = screen.getByRole("region", {
      name: "Where this account stands",
    });
    expect(within(strip).getByText("2 open")).toBeTruthy();
    expect(strip.textContent).toContain("1 stalled");
    // Whose move it is reads the SAME `engagement` field, now in the daily
    // brief rather than a second copy in the strip.
    expect(within(strip).queryByText("Waiting on them")).toBeNull();
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
      name: "Where this account stands",
    });
    // The risk reads the same `state_strip.signal` field, now in the daily
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
      ],
      suggestions_dropped: 0,
    });
    stub(three60);
    renderSuggestions(three60);
    await screen.findByText(/nobody has come back/);

    expect(screen.getByRole("button", { name: "Create draft" })).toBeTruthy();
    // The second rule named no action, so it advises without a control. A
    // button that does nothing teaches the reader to stop pressing them.
    expect(
      screen.queryByRole("button", { name: "Add the next step" }),
    ).toBeNull();
  });

  // The lead panel's footer names what is owed, ported from the retired
  // "Today" card's own commitment tile: a count off `data.length` is a claim
  // about the PAGE, so past the 25-row cap the footer says "25+" rather than
  // a number it cannot stand behind.
  it("says how many are overdue at least, when the open-tasks page is capped", async () => {
    const overdueStep = {
      activity_id: "a-1",
      subject: "Send the renewal paperwork",
      due_at: "2026-05-01T09:00:00Z",
      overdue: true,
      linked_deal_id: null,
      linked_person_id: null,
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

describe("company view — where the record came from", () => {
  // Which of a human, an agent, a connector or nobody wrote a record is the
  // governance reading the provenance tag exists for. Suppressing the reader's
  // OWN hand-typed entry — the one case that reports nothing they do not
  // already know — must not suppress the rest.
  it("names an agent that wrote the record", async () => {
    stub(view(), 200, { ...org, captured_by: "agent:enricher" });
    renderCompany();
    await screen.findByText("Brandt Automotive GmbH");

    await waitFor(() =>
      expect(screen.getAllByText(/enricher/).length).toBeGreaterThan(0),
    );
  });
});

describe("company view — the account's own tabs", () => {
  it("keeps the rail summary beside the People tab", async () => {
    stub(
      view({
        people: {
          data: [
            {
              person_id: "p-1",
              full_name: "Christian Hagemeyer",
              title: "Managing director",
              strength: {
                score: 0,
                bucket: "none",
                factors: {
                  recency: 0,
                  frequency: 0,
                  reciprocity: 0,
                  direction: 0,
                },
              },
              deal_roles: [],
              // A map of purpose → verdict, not a list. The old `[]` was the
              // wrong SHAPE for "no consent recorded", and default-deny is the
              // one reading on this page that must not be guessed at.
              consent: {},
            },
          ],
          page: emptyPage,
        },
      }),
    );
    renderCompany();
    await screen.findByRole("complementary", { name: "Context" });

    await userEvent.click(screen.getByRole("button", { name: /^People/ }));
    // TWICE, deliberately: the tab is the roster in full and the rail's
    // capped summary stands beside it as the reader's anchor across tabs —
    // both checked independently, so the test still fails if either goes
    // missing.
    const rail = await screen.findByRole("complementary", { name: "Context" });
    expect(within(rail).getByText("Christian Hagemeyer")).toBeTruthy();
    expect(screen.getAllByText("Christian Hagemeyer")).toHaveLength(2);
  });

  it("offers the four tabs the record page has, in order", async () => {
    stub(view());
    renderCompany();
    await screen.findByRole("complementary", { name: "Context" });

    // An account with no partner programme still gets all four: Partner is
    // the only conditional tab.
    for (const name of ["Overview", /^People/, "History", "Documents"]) {
      expect(screen.getByRole("button", { name })).toBeTruthy();
    }
    expect(screen.queryByRole("button", { name: "Partner" })).toBeNull();
  });

  it("gives Documents its own tab body", async () => {
    stub(view());
    renderCompany();
    await screen.findByRole("complementary", { name: "Context" });

    await userEvent.click(screen.getByRole("button", { name: "Documents" }));
    // The grid keeps a compact card for "is there paperwork at all"; the tab
    // is the files themselves, so the heading appears twice once it is open.
    await waitFor(() =>
      expect(screen.getAllByText("Documents").length).toBeGreaterThan(1),
    );
  });
});

// The connections card asked nobody and answered everybody: a staff directory
// in the rail of every account, costing a graph read on every page load. The
// route-in asks the question a rep actually has, about one person, and only
// when they ask it.

describe("company view — the account's primary actions", () => {
  it("offers logging what happened and setting what happens next, as separate verbs", async () => {
    stub(view());
    renderCompany();
    await screen.findByRole("complementary", { name: "Context" });

    // One button reading "Log activity", with the task hidden behind a type
    // picker inside it, is why accounts collect notes and no follow-ups. The
    // two verbs answer different questions and are asked separately.
    expect(
      await screen.findByRole("button", { name: "Log activity" }),
    ).toBeTruthy();
    expect(
      await screen.findByRole("button", { name: "Add task" }),
    ).toBeTruthy();
  });

  it("refuses both on an archived company, over one stated reason", async () => {
    stub(view(), 200, { ...org, archived_at: "2026-07-01T09:00:00Z" });
    renderCompany();
    await screen.findByRole("complementary", { name: "Context" });

    // The server refuses a write against a retired record — but REMOVING the
    // verb told a reader nothing, because an absent button reads as a build
    // without the feature. Both are offered and refused, and both point at the
    // one sentence that says what is true of the record.
    const log = await screen.findByRole("button", { name: "Log activity" });
    const task = await screen.findByRole("button", { name: "Add task" });
    expect(log.hasAttribute("disabled")).toBe(true);
    expect(task.hasAttribute("disabled")).toBe(true);

    const reasonId = log.getAttribute("aria-describedby");
    expect(reasonId).toBeTruthy();
    // One fact about the record, said once: the two verbs share the sentence.
    expect(task.getAttribute("aria-describedby")).toBe(reasonId);
    expect(document.getElementById(reasonId ?? "")?.textContent).toContain(
      "archived",
    );
  });
});

// The ONE money reading the customer row carries: the trailing year. Lifetime,
// open balance, overdue and the payment-habit median are Finance-tab readings
// (companyfinance.test.tsx), and this suite pins that the strip stays a glance
// rather than growing a second copy of that card.
describe("a customer's KPI row reports what the account is worth", () => {
  const customer = {
    account: { lifecycle: "customer" as const, relationship_types: [] },
  };
  const connected = (extra: Record<string, unknown>) => ({
    organization_id: "o-1",
    state: "connected",
    provider: "offline_demo",
    ...extra,
  });
  const strip = async () =>
    await screen.findByRole("region", { name: "Where this account stands" });

  // The window the slot names is the window the figure covers. Both figures are
  // on the wire and the lifetime one is the larger, so a slot that reached for
  // the wrong field would read as a spectacularly good year.
  it("shows the trailing year under the trailing year's label", async () => {
    stub(
      view({ state_strip: customer }),
      200,
      org,
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

    expect(within(region).getByText("Net invoiced · 12 mo")).toBeTruthy();
    // Abbreviated: the strip's slots share its width, and a full euro amount
    // wraps mid-number there. The finance card renders the exact figure.
    expect(within(region).getByText(/186(\.4)?K/i)).toBeTruthy();
    // The lifetime figure appears too, and says which window it covers. What
    // this slot must never do is show it AS the trailing year — the two are
    // both on the wire and the lifetime one is the larger, so a slot that
    // reached for the wrong field would read as a spectacularly good year.
    expect(within(region).getByText(/428(\.0)?K lifetime/i)).toBeTruthy();
  });

  // Open balance and overdue are Finance-tab headline readings. A strip that
  // answered them too would be a second copy of that card, read at a glance,
  // drifting from it the first time either changed.
  //
  // The lifetime total is NOT one of them: the Finance tab renders the open
  // balance and the overdue figure and does not carry lifetime anywhere, which
  // is why the strip is the only place the trailing year can be compared
  // against it. Excluding it here was a claim about that card that the card
  // does not support.
  it("does not draw the Finance tab's own readings a second time", async () => {
    stub(
      view({ state_strip: customer }),
      200,
      org,
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

    // No SLOT of its own for any of them — lifetime included; it rides in the
    // money slot's detail line where it cannot be mistaken for the headline.
    expect(within(region).queryByText("Net invoiced · lifetime")).toBeNull();
    expect(within(region).queryByText("Overdue")).toBeNull();
    expect(within(region).queryByText("Open invoices")).toBeNull();
    // The two figures the Finance tab already headlines appear nowhere at all.
    expect(region.textContent).not.toMatch(/34(\.2)?K/i);
    expect(region.textContent).not.toMatch(/12(\.4)?K/i);
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
    await screen.findByRole("region", { name: "Where this account stands" });

  it("names the setup step only when there is no source", async () => {
    stub(view({ state_strip: customer }), 200, org, {
      organization_id: "o-1",
      state: "no_connection",
    });
    renderCompany();
    await strip();
    expect(
      (await screen.findAllByText("Connect your accounting")).length,
    ).toBeGreaterThan(0);
  });

  it("says the source is not matched rather than not connected", async () => {
    stub(view({ state_strip: customer }), 200, org, {
      organization_id: "o-1",
      state: "unmapped",
      provider: "offline_demo",
    });
    renderCompany();
    const region = await strip();
    expect(
      (await screen.findAllByText("Not matched to a customer yet")).length,
    ).toBeGreaterThan(0);
    // The wrong advice, specifically: this reader HAS connected a source.
    expect(region.textContent).not.toMatch(/Connect your accounting/);
  });

  it("says a first sync is running rather than that nothing is connected", async () => {
    stub(view({ state_strip: customer }), 200, org, {
      organization_id: "o-1",
      state: "syncing",
      provider: "offline_demo",
    });
    renderCompany();
    const region = await strip();
    expect((await screen.findAllByText("Syncing…")).length).toBeGreaterThan(0);
    expect(region.textContent).not.toMatch(/Connect your accounting/);
  });

  // A denial and a setup gap are opposite problems. Sending a reader whose
  // role cannot see finance to a settings page asks them to fix the one thing
  // they have no way to fix from there.
  it("says the reading is withheld rather than telling them to set it up", async () => {
    stub(
      view({ state_strip: customer }),
      200,
      org,
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
    expect(
      (await screen.findAllByText("You may not see this account's finance"))
        .length,
    ).toBeGreaterThan(0);
    expect(region.textContent).not.toMatch(/Connect your accounting/);
  });

  it("says the read failed rather than that nothing is connected", async () => {
    stub(
      view({ state_strip: customer }),
      200,
      org,
      { type: "about:blank", title: "Server error", status: 500 },
      500,
    );
    renderCompany();
    const region = await strip();
    expect(
      (await screen.findAllByText("Could not be read")).length,
    ).toBeGreaterThan(0);
    expect(region.textContent).not.toMatch(/Connect your accounting/);
  });

  // `stale` and `error` are opposite claims about whether anything is broken.
  // The contract: stale is a sync that SUCCEEDED long enough ago that the date
  // matters; error is the last good answer after an attempt that FAILED.
  it("calls a stale figure old, not failed", async () => {
    stub(view({ state_strip: customer }), 200, org, {
      organization_id: "o-1",
      state: "stale",
      provider: "offline_demo",
      net_invoiced: { amount_minor: 18642000, currency: "EUR" },
    });
    renderCompany();
    await strip();
    expect((await screen.findAllByText(/186\.4k/)).length).toBeGreaterThan(0);
    expect(
      (await screen.findAllByText(/Last synced a while ago/)).length,
    ).toBeGreaterThan(0);
    expect(screen.queryByText(/sync failed/)).toBeNull();
  });

  // Without this the last good figure renders bare, reading as current.
  it("marks a figure from a failed sync as possibly not current", async () => {
    stub(view({ state_strip: customer }), 200, org, {
      organization_id: "o-1",
      state: "error",
      provider: "offline_demo",
      net_invoiced: { amount_minor: 18642000, currency: "EUR" },
    });
    renderCompany();
    await strip();
    expect((await screen.findAllByText(/186\.4k/)).length).toBeGreaterThan(0);
    expect(
      (await screen.findAllByText(/Last sync failed/)).length,
    ).toBeGreaterThan(0);
  });

  // A live, mapped source that produced no figure is not a missing setup.
  it("says nothing was invoiced rather than telling them to connect", async () => {
    stub(view({ state_strip: customer }), 200, org, {
      organization_id: "o-1",
      state: "connected",
      provider: "offline_demo",
    });
    renderCompany();
    const region = await strip();
    expect(
      (await screen.findAllByText("Nothing invoiced yet")).length,
    ).toBeGreaterThan(0);
    expect(region.textContent).not.toMatch(/Connect your accounting/);
  });

  it("names the source beside a real figure", async () => {
    stub(view({ state_strip: customer }), 200, org, {
      organization_id: "o-1",
      state: "connected",
      provider: "datev",
      net_invoiced: { amount_minor: 18642000, currency: "EUR" },
    });
    renderCompany();
    await strip();
    expect((await screen.findAllByText(/186\.4k/)).length).toBeGreaterThan(0);
    expect((await screen.findAllByText("datev")).length).toBeGreaterThan(0);
  });
});

// The money slot's reason is said ONCE, because there is one slot to say it in,
// and a figure from a window the slot does not name never stands in for one it
// does.
describe("the money slot says its reason once and borrows no figure", () => {
  const customer = {
    account: { lifecycle: "customer" as const, relationship_types: [] },
  };
  const strip = async () =>
    await screen.findByRole("region", { name: "Where this account stands" });

  it("says why there is no figure once, not once per money window", async () => {
    stub(view({ state_strip: customer }), 200, org, {
      organization_id: "o-1",
      state: "unmapped",
      provider: "offline_demo",
    });
    renderCompany();
    const region = await strip();
    await waitFor(() =>
      expect(
        within(region).getAllByText("Not matched to a customer yet").length,
      ).toBe(1),
    );

    // One statement, not three blanks: the other money windows are the Finance
    // tab's, and asking each of them here would repeat one fact about the
    // connection three times across a row that has to stay one line.
    expect(within(region).queryByText("Net invoiced · lifetime")).toBeNull();
    expect(within(region).queryByText("Overdue")).toBeNull();
  });

  // The lifetime total is the figure most likely to be present when the
  // trailing year is not — an account billed years ago and nothing since. Shown
  // under the trailing year's label it would report a year that never happened,
  // so the slot must refuse and say why instead.
  it("refuses rather than showing a lifetime total as the trailing year", async () => {
    stub(view({ state_strip: customer }), 200, org, {
      organization_id: "o-1",
      state: "unmapped",
      provider: "offline_demo",
      net_invoiced_lifetime: { amount_minor: 42800000, currency: "EUR" },
    });
    renderCompany();
    const region = await strip();
    await waitFor(() => expect(region.textContent).not.toMatch(/Loading…/));

    // No figure at all, and the reason named: the lifetime total on the wire is
    // not this slot's answer, and a dash with no reason would not be either.
    expect(region.textContent).not.toMatch(/428(\.0)?K/i);
    expect(
      within(region).getAllByText("Not matched to a customer yet").length,
    ).toBe(1);
  });

  // The team lead's two mandated shapes: a customer whose finance connection
  // cannot answer, and one whose connection can. Both must still carry the
  // account's own standing (pipeline, relationship, health) — the defect this
  // whole suite exists for was the standing readings disappearing behind the
  // money, not the money itself.
  // A relationship dimension and a live reply balance are enough to draw
  // both HealthStat ("Conversation") and HealthSummaryStat ("Health") with real
  // verdicts rather than their unassessed readings, which is what makes the
  // standing half of the row worth asserting here.
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
    stub(withStanding(), 200, org, {
      organization_id: "o-1",
      state: "unmapped",
      provider: "offline_demo",
    });
    renderCompany();
    const region = await strip();
    await waitFor(() =>
      expect(
        within(region).getAllByText("Not matched to a customer yet").length,
      ).toBe(1),
    );

    expect(within(region).getByText("Open pipeline")).toBeTruthy();
    // The card's own label: the tile reads the correspondence and is named
    // Conversation, while the health receipt still names a Relationship
    // dimension of its own.
    expect(
      within(region).getByText("Conversation", {
        selector: ".stat-card-label",
      }),
    ).toBeTruthy();
    expect(within(region).getByText("Health")).toBeTruthy();
    expect(within(region).getByText("Finance")).toBeTruthy();
  });

  it("shows the real figure beside the same standing readings", async () => {
    stub(withStanding(), 200, org, {
      organization_id: "o-1",
      state: "connected",
      provider: "datev",
      net_invoiced: { amount_minor: 18642000, currency: "EUR" },
      net_invoiced_lifetime: { amount_minor: 42800000, currency: "EUR" },
      overdue: { amount_minor: 1243000, currency: "EUR" },
    });
    renderCompany();
    const region = await strip();
    await waitFor(() => expect(region.textContent).not.toMatch(/Loading…/));

    expect(within(region).getByText("Open pipeline")).toBeTruthy();
    // The card's own label: the tile reads the correspondence and is named
    // Conversation, while the health receipt still names a Relationship
    // dimension of its own.
    expect(
      within(region).getByText("Conversation", {
        selector: ".stat-card-label",
      }),
    ).toBeTruthy();
    expect(within(region).getByText("Health")).toBeTruthy();
    expect(within(region).getByText("Net invoiced · 12 mo")).toBeTruthy();
    expect(within(region).getByText(/186(\.4)?K/i)).toBeTruthy();
    // "Finance" is the label of a slot that has nothing to report, so it must
    // not stand over a figure — the label names which reading the value is.
    expect(within(region).queryByText("Finance")).toBeNull();
  });
});
