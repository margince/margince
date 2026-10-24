/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import type { AccountScan } from "./accountscan";
import { NeedsList, useTodayReading } from "./companytoday";

// The section earns its place by carrying what nothing else on the page says,
// and by never stating a claim it cannot support. Both are testable.

afterEach(cleanup);

type Company360 = components["schemas"]["Company360"];

// A COMPLETE Company360, not a cast one. A fixture asserted into the
// contract type can drop a required field or carry an invalid value and still
// compile, so the test would go on passing after the wire shape moved under it.
const BASE: Company360 = {
  as_of: "2026-08-07T09:00:00Z",
  company: {
    id: "o-1",
    display_name: "Acme",
    source: "manual",
    captured_by: "human:test",
    created_at: "2026-08-01T09:00:00Z",
    updated_at: "2026-08-01T09:00:00Z",
  },
  sections_omitted: [],
};

type ShowOptions = {
  loading?: boolean;
  failed?: boolean;
  onDraftTo?: (contactId: string) => void;
  onOpenRecord?: (entityType: string, entityId: string) => void;
  onPrepareMeeting?: (activityId: string) => void;
  scan?: AccountScan;
};

// The hook and the list the Overview composes, without the Overview's own
// fetch of the scan. These cases hand the scan in, in every state it has.
function Needs({ view, opts }: { view?: Company360; opts: ShowOptions }) {
  const reading = useTodayReading({
    companyId: "o-1",
    view,
    loading: opts.loading ?? false,
    failed: opts.failed ?? false,
    onDraftTo: opts.onDraftTo,
    onOpenRecord: opts.onOpenRecord,
    onPrepareMeeting: opts.onPrepareMeeting,
    scan: opts.scan,
  });
  return <NeedsList reading={reading} />;
}

function show(view?: Company360, opts: ShowOptions = {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <Needs view={view} opts={opts} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// The withheld line, found by the words the catalog puts BEFORE the list of
// sections. Which sections it names is what these cases are about, and a copy
// of the whole sentence here would fail on a rewrite that changed nothing
// about the reading.
const withheldPrefix = en["today.withheld"].slice(
  0,
  en["today.withheld"].indexOf("{sections}"),
);
const withheldLine = () =>
  screen.getByText((content) => content.startsWith(withheldPrefix));

describe("what needs a contact on this account today", () => {
  it("says nothing about a meeting when none is booked", () => {
    // Absent AND not named in sections_omitted means "none scheduled". Writing
    // a line about it would be missing data dressed as a recommendation — only
    // the suggestion engine can name WHOM to contact, so only it may advise
    // booking one.
    show(BASE);
    expect(screen.getByText(en["today.quiet"])).toBeTruthy();
    expect(
      screen.queryByText((content) => content.startsWith(withheldPrefix)),
    ).toBeNull();
  });

  it("says the calendar is hidden when the reader has no activity grant", () => {
    // The same ABSENT field, opposite meaning. Without sections_omitted a
    // client would tell someone with no calendar access to book a meeting that
    // already exists.
    show({
      ...BASE,
      sections_omitted: ["next_meeting"],
    });
    expect(withheldLine().textContent).toContain(
      en["today.source.nextMeeting"],
    );
  });

  it("says a source is hidden from the reader rather than composing a shorter list silently", () => {
    show({
      ...BASE,
      sections_omitted: ["next_meeting", "next_steps"],
    });

    // "Not included", never "None": a list assembled from three of five
    // sources is not the same list, and only the reader can judge whether the
    // missing one mattered.
    const withheld = withheldLine();
    expect(withheld.textContent).toContain(en["today.source.nextMeeting"]);
    expect(withheld.textContent).toContain(en["today.source.nextSteps"]);
  });

  // The interaction tile reads the activities section, so a caller with no
  // activity grant must be TOLD the tile is missing. Without the section in
  // the footer's list it would vanish in silence, which reads as an account
  // nobody has spoken to.
  it("names the activities section when the reader may not see what was said", () => {
    show({ ...BASE, sections_omitted: ["activities"] });

    expect(withheldLine().textContent).toContain(en["today.source.activities"]);
    expect(screen.queryByText("Last exchange")).toBeNull();
  });

  // Whose move it is and the open-risk tile both read `state_strip`
  // (whoseMove and openRisk). The KPI strip's own withheld rendering is
  // tested elsewhere (company360.test.tsx), but that is the STRIP's read of
  // the same fields — this brief reads them independently, off its own copy
  // of the view, and had no coverage of its own withheld path until now.
  it("names both readings when the reader may not see whose move it is or what is at risk", () => {
    show({ ...BASE, sections_omitted: ["state_strip"] });

    expect(withheldLine().textContent).toContain(en["today.source.standing"]);
  });

  // The best-route tile reads `contacts`. A caller scoped away from the
  // roster must be told the reading is missing, not shown a brief that
  // silently never names a way in.
  it("names the contacts when the reader may not see who is here", () => {
    show({ ...BASE, sections_omitted: ["contacts"] });

    expect(withheldLine().textContent).toContain(en["today.source.contacts"]);
  });

  it("distinguishes a failed read from a quiet account", () => {
    show(undefined, { failed: true });
    // "This section couldn't load" and "nothing needs you" are different
    // sentences, and only one of them is about the account.
    expect(screen.getByText(en["today.failed"])).toBeTruthy();
    expect(screen.queryByText(en["today.quiet"])).toBeNull();
  });

  // The account brief's own footer reports this with the baseline it counted
  // from. A second, shorter copy here is the duplication this section's rules
  // forbid, so what changed since the last visit earns no tile.
  it("leaves what changed since the last visit to the brief that reports it", () => {
    show({
      ...BASE,
      since_last_visit: {
        new_activities: 3,
        baseline_at: "2026-08-01T09:00:00Z",
      },
    });
    expect(screen.getByText(en["today.quiet"])).toBeTruthy();
  });

  it("reports the failure even when a view is in hand", () => {
    // show(undefined, {failed:true}) passes on the missing view alone, so it
    // cannot tell `if (failed || !view)` from `if (!view)`. This one can: the
    // view is present and quiet, and the failure still has to win.
    show(BASE, { failed: true });

    expect(screen.getByText(en["today.failed"])).toBeTruthy();
    expect(screen.queryByText(en["today.quiet"])).toBeNull();
  });
});

// The list's own moves and the foot under them. The rules are choices rather
// than derivations, so each is pinned here.
describe("the moves on the list, and the foot under them", () => {
  // The contract requires the full factor breakdown on every strength; the
  // tiles read only the score, but a fixture that omits them is not the shape
  // the wire sends.
  const FACTORS = { recency: 0, frequency: 0, reciprocity: 0, direction: 0 };
  const CONTACT = {
    contact_id: "p-1",
    full_name: "Sarah Cole",
    strength: { score: 40, bucket: "moderate" as const, factors: FACTORS },
    deal_roles: [],
    consent: {},
  };

  // What is owed lives in the panel's footer (nextCommitmentLine, in
  // company360.suggestions.tsx), not in a context tile. The subject stays out
  // of it. The next-steps card renders the subject with a due-date edit and a
  // complete button. A flat copy here would be the weaker of the two.
  it("says how many are overdue in the footer, never the subject", () => {
    show({
      ...BASE,
      next_steps: {
        data: [
          {
            activity_id: "a-1",
            subject: "Send the revised proposal",
            due_at: "2026-08-05T09:00:00Z",
            overdue: true,
          },
          { activity_id: "a-2", subject: "Later thing", overdue: false },
        ],
        page: { has_more: false, next_cursor: null },
      },
    });
    expect(screen.getByText("1 overdue")).toBeTruthy();
    expect(screen.queryByText("Send the revised proposal")).toBeNull();
    expect(screen.queryByText("Later thing")).toBeNull();
  });

  it("says how many are open when none is overdue", () => {
    show({
      ...BASE,
      next_steps: {
        data: [{ activity_id: "a-1", subject: "Someday", overdue: false }],
        page: { has_more: false, next_cursor: null },
      },
    });
    expect(screen.getByText("1 open")).toBeTruthy();
  });

  // The route rule: strongest CONTACT, then that contact's strongest ROUTE.

  // The largest-open-deal reading belongs to the Commercial panel
  // (companies.tabs.tsx), beside the full open-deals list, so this file picks
  // and ranks no deal of its own.

  // The button names the recipient it will write to, and hands that contact to
  // the composer: an account-started message has no thread to anchor on, so
  // the recipient is what grounds it.
  it("hands the named recipient to the composer", () => {
    const drafted = vi.fn();
    show(
      {
        ...BASE,
        contacts: {
          data: [
            {
              ...CONTACT,
              routes: {
                top: [
                  {
                    user_id: "u-1",
                    display_name: "Lars",
                    strength_bucket: "strong" as const,
                  },
                ],
                remainder: 0,
                untried: false,
              },
            },
          ],
          page: { has_more: false, next_cursor: null },
        },
      },
      { onDraftTo: drafted },
    );
    fireEvent.click(screen.getByRole("button", { name: "Draft" }));
    expect(drafted).toHaveBeenCalledWith(CONTACT.contact_id);
  });

  // The MOVES half of the merged brief: a booked meeting's own verb renders
  // as a full-bleed row alongside whatever advice the account has, rather
  // than as a sidebar button beside the context tiles.
  it("offers to prepare for a booked meeting as a move row", () => {
    const prepared = vi.fn();
    show(
      {
        ...BASE,
        next_meeting: {
          activity_id: "a-1",
          starts_at: "2026-08-12T09:00:00Z",
          subject: "Renewal review",
          participants: [{ contact_id: "p-1", display_name: "Dana Buyer" }],
        },
      },
      { onPrepareMeeting: prepared },
    );
    // The verb names what the button does: it opens the meeting brief for
    // this room. It read "Write to the room" while the account page had no
    // brief drawer and the handler opened the composer instead — the label
    // and the handler moved together (aiscope.test.tsx pins the
    // drawer).
    fireEvent.click(screen.getByRole("button", { name: "Prepare meeting" }));
    expect(prepared).toHaveBeenCalledWith("a-1");
  });

  it("carries the account's suggestions as moves alongside the context band", () => {
    show({
      ...BASE,
      suggestions: [
        {
          kind: "no_reply",
          fingerprint: "f-1",
          reason: "You reached out 15 days ago and nobody has come back.",
          evidence: [],
        },
      ],
    });
    expect(screen.getByText(/nobody has come back/)).toBeTruthy();
    expect(screen.getByRole("button", { name: "Not now" })).toBeTruthy();
  });
});

// Every 360 collection is a page of 25 with `has_more` beside it. A reading
// that counts off that page states a fact about the PAGE, and the reader has
// no way to tell.
describe("a page is not the account", () => {
  it("says 25+ overdue rather than a count it cannot stand behind", () => {
    show({
      ...BASE,
      next_steps: {
        data: [
          {
            activity_id: "a-1",
            subject: "One of many",
            due_at: "2026-08-05T09:00:00Z",
            overdue: true,
          },
        ],
        page: { has_more: true, next_cursor: "c" },
      },
    });
    expect(screen.getByText("1+ overdue")).toBeTruthy();
    expect(screen.queryByText("1 overdue")).toBeNull();
  });
});

// The scan on the needs list: the pending row while Margince reads, the
// merged list once it has answered, and the foot that says who wrote it.
describe("the account scan on the needs list", () => {
  const ruleRow = {
    kind: "no_reply" as const,
    fingerprint: "f-1",
    reason: "You reached out 15 days ago and nobody has come back.",
    evidence: [],
  };
  const modelRow = {
    kind: "question_unanswered" as const,
    fingerprint: "f-2",
    title: "Confirm the depot installation",
    reason: "Dana asked whether installation can happen at their depot.",
    written_by: "model" as const,
    evidence: [
      {
        entity_type: "activity" as const,
        entity_id: "a-3",
        name: "Re: proposal",
        quote:
          "can you confirm whether the installation can be done at our depot",
        at: "2026-08-03T14:20:00Z",
        origin: "Email they sent",
      },
    ],
  };

  it("draws the pending row above the rules' rows while Margince reads", () => {
    show(
      { ...BASE, suggestions: [ruleRow] },
      {
        scan: {
          company_id: "o-1",
          state: "running",
          findings: [ruleRow],
          findings_dropped: 0,
        },
      },
    );
    expect(
      screen.getByText(
        "Margince is analyzing this company’s exchanges and deals.",
      ),
    ).toBeTruthy();
    expect(screen.getByText(/nobody has come back/)).toBeTruthy();
  });

  it("draws the merged list once the read has answered, with the read's own foot", () => {
    show(
      { ...BASE, suggestions: [ruleRow] },
      {
        scan: {
          company_id: "o-1",
          state: "done",
          generated_at: "2026-08-07T08:58:00Z",
          generated_by: "model",
          read: { exchanges: 14, deals: 2 },
          stale: true,
          findings: [ruleRow, modelRow],
          findings_dropped: 0,
        },
      },
    );
    expect(screen.getByText("Confirm the depot installation")).toBeTruthy();
    expect(screen.getByText(/nobody has come back/)).toBeTruthy();
    expect(screen.queryByRole("status")).toBeNull();
    expect(screen.getByText("Written by Margince")).toBeTruthy();
    expect(screen.getByText("Analyzed 14 exchanges and 2 deals")).toBeTruthy();
    expect(
      screen.getByText(
        "The company has changed since. It is analyzed again within the hour.",
      ),
    ).toBeTruthy();
  });

  it("says when a read the budget put off resumes", () => {
    show(
      { ...BASE, suggestions: [ruleRow] },
      {
        scan: {
          company_id: "o-1",
          state: "queued",
          resumes_at: "2026-08-07T09:30:00Z",
          degrade_reason: "budget_deferred",
          findings: [ruleRow],
          findings_dropped: 0,
        },
      },
    );
    expect(
      screen.getByText(/Analysis resumes .*Paused by the AI budget/),
    ).toBeTruthy();
  });

  it.each([
    ["provider_deferred", /Analysis retries .*Waiting for the AI provider/],
    ["future_reason", /Analysis resumes/],
    [undefined, /Analysis resumes/],
  ])("does not blame the budget for %s", (reason, expected) => {
    show(
      { ...BASE, suggestions: [ruleRow] },
      {
        scan: {
          company_id: "o-1",
          state: "queued",
          resumes_at: "2026-08-07T09:30:00Z",
          degrade_reason: reason,
          findings: [ruleRow],
          findings_dropped: 0,
        },
      },
    );
    expect(screen.getByText(expected)).toBeTruthy();
    expect(screen.queryByText(/Paused by the AI budget/)).toBeNull();
  });

  it("counts one exchange and one deal in the singular", () => {
    show(
      { ...BASE, suggestions: [ruleRow] },
      {
        scan: {
          company_id: "o-1",
          state: "done",
          generated_at: "2026-08-07T08:58:00Z",
          generated_by: "model",
          read: { exchanges: 1, deals: 1 },
          findings: [ruleRow],
          findings_dropped: 0,
        },
      },
    );
    expect(screen.getByText("Analyzed 1 exchange and 1 deal")).toBeTruthy();
  });

  it("says why the model did not write the rows when the read degraded", () => {
    show(
      { ...BASE, suggestions: [ruleRow] },
      {
        scan: {
          company_id: "o-1",
          state: "degraded",
          generated_at: "2026-08-07T08:58:00Z",
          generated_by: "deterministic",
          degrade_reason:
            "No model is configured here, so only the rule-based advice is shown.",
          findings: [ruleRow],
          findings_dropped: 0,
        },
      },
    );
    expect(screen.getByText(/No model is configured here/)).toBeTruthy();
    expect(screen.queryByText("Written by Margince")).toBeNull();
  });
});

// The account offers ONE way to write to somebody, not two that disagree.
//
// The generic row picks the account's strongest contact; a `draft_reply`
// suggestion names the contact actually waiting on an answer. On the demo
// account those are two different contacts, so drawing both told a rep to write
// to Sarah while the advice above said to answer Frédéric.
describe("the generic draft row", () => {
  const RECIPIENT = {
    contact_id: "p-strongest",
    full_name: "Sarah Cole",
    strength: {
      score: 40,
      bucket: "moderate" as const,
      factors: { recency: 0, frequency: 0, reciprocity: 0, direction: 0 },
    },
    deal_roles: [],
    consent: {},
  };
  const withRecipient = {
    ...BASE,
    contacts: {
      data: [RECIPIENT],
      page: { has_more: false, next_cursor: null },
    },
  } satisfies Company360;

  it("stands on its own when no advice names a message to answer", () => {
    show(withRecipient, { onDraftTo: vi.fn() });

    // The admit case. Without it the absence below would pass over a row that
    // never renders at all.
    expect(screen.getByText(en["today.draft.new"])).toBeTruthy();
  });

  it("gives way to a suggestion that names the message to answer", () => {
    show(
      {
        ...withRecipient,
        suggestions: [
          {
            kind: "no_reply",
            fingerprint: "f-reply",
            reason: "Frédéric asked about fallbacks and nobody answered.",
            evidence: [],
            action: { kind: "draft_reply", activity_id: "act-1" },
          },
        ],
      },
      { onDraftTo: vi.fn() },
    );

    expect(screen.queryByText(en["today.draft.new"])).toBeNull();
    expect(screen.getByText(/nobody answered/)).toBeTruthy();
  });

  it("links the recipient's name to their own page", () => {
    show(withRecipient, { onDraftTo: vi.fn() });

    // The name used to be plain text inside the row's label, so a reader who
    // wanted to know who Sarah Cole is had nowhere to press.
    expect(screen.getByRole("link", { name: "Sarah Cole" })).toBeTruthy();
  });
});
