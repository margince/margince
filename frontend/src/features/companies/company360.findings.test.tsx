/** @vitest-environment happy-dom */
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TaskQuickActions, useTaskUpdate } from "../../components/taskactions";
import { taskWriteKeys } from "../../lib/activitykeys";
import { SentenceList } from "../records/record360";
import { CompanyScreen } from "./companies";
import {
  type Company360,
  company,
  emptyPage,
  partnerCompany,
  render,
  renderCompany,
  renderSuggestions,
  stub,
  view,
} from "./company360.testkit";
import { CompanyTaskTable } from "./companytaskstable";

beforeEach(() => {
  globalThis.localStorage.setItem("margince.workspaceSlug", "acme");
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  globalThis.localStorage.clear();
});

// The Tasks tab's list plus the per-row verbs, which it takes as a render slot
// rather than owning. Mounting the two together pins the pairing under test. A
// row offers Done always, and Snooze only when there is a date to move.
function NextStepsWithVerbs({ three60 }: Readonly<{ three60: Company360 }>) {
  const update = useTaskUpdate(taskWriteKeys("company", "o-1"));
  return (
    <CompanyTaskTable
      view={three60}
      onOpenTask={() => {}}
      renderAction={(step) => (
        <TaskQuickActions
          activityId={step.activity_id}
          version={step.version}
          dueAt={step.due_at}
          update={update}
        />
      )}
    />
  );
}

// The company page's own affordances: what it says is waiting, and what a
// reader can do about it without leaving the account.

describe("company view — the citations under a finding", () => {
  // The chips are shared by every grounded surface on this page. They are
  // exercised through the advice the brief carries, which is where a reader
  // meets them. The citations are typed from the contract rather than taken as
  // `unknown[]`. An `entity_type` the server does not serve is the drift
  // these chips would go on rendering.
  type Suggestion = NonNullable<Company360["suggestions"]>[number];
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

  // The page's rule: a citation reads as the record's name
  // wherever the page holds one. The server names what its writer had at hand;
  // this account's 360 holds the rest.
  it("names the record from the account's own roster when the citation does not", async () => {
    const three60 = view({
      contacts: {
        data: [
          {
            contact_id: "p-9",
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
      suggestions: [suggestion([{ entity_type: "contact", entity_id: "p-9" }])],
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

  // The dossier's "collected" mode gathers every sentence's citations under one
  // row. There, several profile fields (a kind with no name beyond "profile
  // field") must print as one counted button, not as ten identical ones.
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
        onOpenReceipt={(...args) => opened.push(args)}
        citations="collected"
      />,
    );
    expect(screen.getByText("3 profile fields")).toBeTruthy();
    expect(screen.queryAllByText("profile field")).toHaveLength(0);
    // Opens the first record, with every sibling, so the drawer's own stepper
    // reaches the other two. The count names them; the stepper reaches them.
    await userEvent.click(screen.getByText("3 profile fields"));
    expect(opened).toEqual([
      [
        { entityType: "profile_field", entityId: "pf-1" },
        [
          { entityType: "profile_field", entityId: "pf-1" },
          { entityType: "profile_field", entityId: "pf-2" },
          { entityType: "profile_field", entityId: "pf-3" },
        ],
      ],
    ]);
  });

  // deal/contact each open their own screen rather than a shared stepper.
  // Grouping them the same way would drop every record past the first, so they
  // stay one chip per record.
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

describe("company view — what is waiting on a decision stays off the page", () => {
  // `status` is a closed union on the wire, so it is annotated rather than
  // inferred as `string`. A widened fixture could name a status the server
  // never sends, and the card would go on drawing it.
  type StagedApproval = NonNullable<
    Company360["pending_approvals"]
  >["data"][number];
  const staged: StagedApproval = {
    id: "ap-1",
    kind: "site_lead",
    status: "pending",
    summary: "Add Markus Bueckle as a contact",
    proposed_change: { full_name: "Markus Bueckle" },
    proposed_by: "agent:capture",
    target_entity_type: "company",
    target_entity_id: "o-1",
    diff_hash: "h1",
    created_at: "2026-06-01T08:00:00Z",
    evidence: [],
  };

  // The design's record menu has no queue entry. The queue itself is the
  // brief's decisions list. A proposal waiting on this account is decided
  // there rather than in a panel on the account.
  it("offers no queue on the account, however many proposals wait", async () => {
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
    await screen.findByText("Brandt Automotive GmbH");
    (await screen.findByRole("button", { name: "More actions" })).click();
    await screen.findByTestId("company-full-history");
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
    linked_contact_id: null,
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
    expect(screen.getByRole("button", { name: "Snooze 1 day" })).toBeTruthy();
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
    expect(screen.queryByRole("button", { name: "Snooze 1 day" })).toBeNull();
  });

  it("draws no checkbox when the caller has no write path for it", async () => {
    const three60 = view({ next_steps: { data: [step], page: emptyPage } });
    stub(three60);
    render(<CompanyTaskTable view={three60} />);
    await waitFor(() =>
      expect(screen.getByText("Send the retrofit proposal")).toBeTruthy(),
    );
    expect(screen.queryByRole("checkbox")).toBeNull();
  });
});

// The page said "nobody here is your champion" and gave the reader nowhere to
// say who is. The roles live on relationship rows written from the deal screen.
// The warning was true, unactionable and permanent.

// A role belongs to a deal, so the same contact can be champion on one and
// nobody on another. Rendering the role alone made two clauses that read
// identically.

describe("company view — Partner is not a permanent tab", () => {
  it("offers the account's own tabs but not Partner on an account with no programme", async () => {
    stub(view());
    renderCompany();
    await screen.findByRole("complementary", { name: "Context" });

    // 360, Contacts and History belong to every account. Partner is a form
    // about a commercial arrangement almost none of them have.
    expect(screen.getByRole("button", { name: "Overview" })).toBeTruthy();
    expect(screen.getByRole("button", { name: /^Contacts/ })).toBeTruthy();
    expect(screen.getByRole("button", { name: /^History/ })).toBeTruthy();
    expect(screen.queryByRole("button", { name: "Partner" })).toBeNull();
  });

  it("shows both tabs once the account has a programme", async () => {
    stub(view(), 200, partnerCompany);
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
      screen.getByRole("button", { name: "Set up partner program" }),
    );

    // Asking for the form is what puts the tab on screen. Without it, the
    // hidden tab would leave the first partner row unreachable.
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

describe("company view — where the record came from", () => {
  // Which of a human, an agent, a connector or nobody wrote a record is the
  // governance reading the provenance tag exists for. Hiding the reader's own
  // hand-typed entry (the one case that tells them nothing new) must not hide
  // the rest.
  it("names an agent that wrote the record", async () => {
    stub(view(), 200, { ...company, captured_by: "agent:enrich" });
    renderCompany();
    await screen.findByText("Brandt Automotive GmbH");

    await waitFor(() =>
      expect(screen.getAllByText(/Signature reader/).length).toBeGreaterThan(0),
    );
  });
});

describe("company view — the account's own tabs", () => {
  it("lists the roster once, on the Contacts tab", async () => {
    stub(
      view({
        contacts: {
          data: [
            {
              contact_id: "p-1",
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
              // A map of purpose → verdict, not a list. A `[]` is the wrong
              // shape for "no consent recorded", and default-deny is the
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

    await userEvent.click(screen.getByRole("button", { name: /^Contacts/ }));
    // The details beside the tab carry the best route in, never a second
    // copy of the roster the tab already lists in full.
    expect(await screen.findByText("Christian Hagemeyer")).toBeTruthy();
    const rail = screen.getByRole("complementary", { name: "Context" });
    expect(within(rail).queryByText("Christian Hagemeyer")).toBeNull();
  });

  it("offers the four tabs the record page has, in order", async () => {
    stub(view());
    renderCompany();
    await screen.findByRole("complementary", { name: "Context" });

    // An account with no partner programme still gets all four: Partner is
    // the only conditional tab.
    for (const name of ["Overview", /^Contacts/, /^History/, "Documents"]) {
      expect(screen.getByRole("button", { name })).toBeTruthy();
    }
    expect(screen.queryByRole("button", { name: "Partner" })).toBeNull();
  });

  it("gives Documents its own tab body", async () => {
    stub(view());
    renderCompany();
    await screen.findByRole("complementary", { name: "Context" });

    await userEvent.click(screen.getByRole("button", { name: "Documents" }));
    // The grid keeps a compact card for "is there paperwork at all". The tab is
    // the files themselves, so the heading appears twice once it is open.
    await waitFor(() =>
      expect(screen.getAllByText("Documents").length).toBeGreaterThan(1),
    );
  });
});

// A connections card asks nobody and answers everybody: a staff directory in
// the rail of every account, costing a graph read on every page load. The
// route-in asks the question a rep has, about one contact, and only when they
// ask it.

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
    stub(view(), 200, { ...company, archived_at: "2026-07-01T09:00:00Z" });
    renderCompany();
    await screen.findByRole("complementary", { name: "Context" });

    // The server refuses a write against a retired record. Removing the verb
    // would tell a reader nothing, because an absent button reads as a build
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
