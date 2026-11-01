/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
} from "../../testing/story-utils";
import { ContactToday } from "./contacttoday";

// The Next bar gives one answer, and gives it once. It answers "what is next"
// on every contact. So the quiet rung and the thin relationship are answers
// inside it, not a reason to draw nothing. The one thing the move is about is
// named once, not again in the "+N tasks" count.
//
// Assertions read the catalog rather than quoting English: the sentences here
// are product copy and a test pinning them fails on a rewrite that changed
// nothing about the behaviour.
//
// Mounted on the card rather than through the whole page. The page's own suite
// is at its length ceiling, and the bar's composition under test here needs
// neither the header nor the rail.

beforeEach(() =>
  installFetchStub({
    "GET /me": meRoute({ activity: ["read"] }),
    "GET /activities/a-9": () => jsonResponse(OPEN_TASK),
  }),
);
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

type Contact360 = components["schemas"]["Contact360"];
type ContactMoment = components["schemas"]["ContactMoment"];
type Activity = components["schemas"]["Activity"];
type ContactPhone = components["schemas"]["ContactPhone"];

const CAPTURED = {
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
} as const;

// A COMPLETE Contact360, not a cast one: a fixture asserted into the contract
// type can drop a required field and still compile, and the case would go on
// passing after the wire shape moved under it.
const VIEW: Contact360 = {
  as_of: "2026-08-13T09:00:00Z",
  contact: { id: "p-1", full_name: "Dana Buyer", ...CAPTURED },
  sections_omitted: [],
};

// Rung 10, as the server sends it: a verb, no evidence, and a headline that
// speaks for the whole ladder.
const QUIET: ContactMoment = {
  claim_key: "moment:nothing_needed",
  evidence_fingerprint: "quiet",
  rule: "nothing_needed",
  headline: "Nothing needs you today",
  why_now: "No meeting coming up, nothing owed, nobody waiting on a reply.",
  confidence: "observed_fact",
  evidence: [],
  recommended_action: {
    kind: "log_activity",
    label: "Log an interaction",
    state: "available",
    destination: { surface: "activity_log" },
  },
};

const THIN: ContactMoment = {
  ...QUIET,
  claim_key: "moment:thin_relationship",
  rule: "thin_relationship",
  headline: "No interactions recorded",
  why_now: "Nothing has been written down about this contact yet.",
};

const OPEN_TASK = {
  offers_email: false,
  id: "a-9",
  kind: "task",
  subject: "Send the renewal quote",
  occurred_at: "2026-08-10T09:00:00Z",
  occurred_precision: "instant",
  is_done: false,
  ...CAPTURED,
} as const;

const OTHER_TASK = {
  ...OPEN_TASK,
  id: "a-8",
  subject: "Book the site visit",
} as const;

// A promise, as the ladder sends one: the headline is written FROM the task,
// so the same commitment is the ask, the record it rests on and a row on the
// list unless the card says it once.
const PROMISE: ContactMoment = {
  claim_key: "moment:open_promise",
  evidence_fingerprint: "promise",
  rule: "open_promise",
  headline: "You owe them: Send the renewal quote",
  why_now: "A commitment with a date on it, still open.",
  confidence: "observed_fact",
  evidence: [{ type: "task", id: "a-9", label: "Send the renewal quote" }],
  recommended_action: {
    kind: "complete_task",
    label: "Open it",
    state: "available",
  },
};

function show(view: Contact360, moment?: ContactMoment) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ContactToday
          view={view}
          moment={moment}
          onAction={() => {}}
          onOpenTasks={() => {}}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return client;
}

describe("the move the panel leads with", () => {
  it("asks for THIS contact's move and gives the server's own reason", () => {
    show(VIEW, PROMISE);

    expect(screen.getByText(PROMISE.headline)).toBeTruthy();
    // The server's reason and no other: the facts in it are what a rep
    // judges, and the account brief reads the same moment the same way.
    expect(screen.getByText(PROMISE.why_now)).toBeTruthy();
  });

  it("says how late the promise it is about is, against the read's own date", () => {
    show(
      {
        ...VIEW,
        next_steps: {
          data: [{ ...OPEN_TASK, due_at: "2026-08-03T09:00:00Z" }],
          page: { has_more: false },
        },
      },
      PROMISE,
    );

    expect(screen.getByText("10 days late")).toBeTruthy();
  });

  it("names the task it is about once, not as a chore under itself too", () => {
    show(
      {
        ...VIEW,
        next_steps: {
          data: [OPEN_TASK, OTHER_TASK],
          page: { has_more: false },
        },
      },
      PROMISE,
    );

    expect(screen.getByText(PROMISE.headline)).toBeTruthy();
    // The named task is the ask; only the other one is counted behind it.
    expect(
      screen.getByRole("button", {
        name: en["standing.next.more_one"].replace("{count}", "1"),
      }),
    ).toBeTruthy();
  });

  it("shows what it rests on only where that is a record the ask does not name", () => {
    show(VIEW, PROMISE);
    expect(screen.queryByText(en["co.suggest.basedOn"])).toBeNull();

    cleanup();
    show(VIEW, {
      ...PROMISE,
      evidence: [
        { type: "activity", id: "a-1", label: "Re: the retrofit timeline" },
      ],
    });
    expect(screen.getByText(en["co.suggest.basedOn"])).toBeTruthy();
    expect(screen.getByText("Re: the retrofit timeline")).toBeTruthy();
  });
});

describe("the day's work on a contact", () => {
  // The ladder reaches rung 10 with work still on the page whenever the reader
  // has dismissed the card that spoke for it: a dismissal silences a card, not
  // the record. The task stays listed, and the panel must not answer "nothing
  // needs you today" over the top of it.
  it("drops the quiet card when an open task is still listed", () => {
    show(
      {
        ...VIEW,
        next_steps: { data: [OPEN_TASK], page: { has_more: false } },
      },
      QUIET,
    );

    expect(screen.getByText(OPEN_TASK.subject)).toBeTruthy();
    expect(screen.queryByText(QUIET.headline)).toBeNull();
  });

  it("shows the task's own words as one plain line under its subject", () => {
    const worded = {
      ...OPEN_TASK,
      body: "**Send** the fleet numbers\n\nThen book the pricing call.",
    };
    show(
      { ...VIEW, next_steps: { data: [worded], page: { has_more: false } } },
      QUIET,
    );

    expect(screen.getByText("Send the fleet numbers")).toBeTruthy();
    expect(screen.queryByText(/Then book the pricing call/)).toBeNull();
  });

  it("shows no words for a task the reader may not read", () => {
    const withheld = {
      ...OPEN_TASK,
      body: "Send the fleet numbers",
      content_state: "withheld" as const,
    };
    show(
      { ...VIEW, next_steps: { data: [withheld], page: { has_more: false } } },
      QUIET,
    );

    expect(screen.getByText(OPEN_TASK.subject)).toBeTruthy();
    expect(screen.queryByText("Send the fleet numbers")).toBeNull();
  });

  // The quiet rung is the answer to the bar's question, so it is answered
  // where the question is asked. Rendering nothing at all left a reader
  // unable to tell a quiet day from a pane that failed to load.
  it("answers a quiet record inside the bar", () => {
    show(VIEW, QUIET);

    expect(screen.getByText(en["standing.next.none"])).toBeTruthy();
    // Nobody suggested that nothing needs doing, and there is nothing to press
    // about it.
    expect(screen.queryByText(en["co.suggest.byline"])).toBeNull();
    expect(screen.queryByRole("button")).toBeNull();
  });

  // A relationship with nothing recorded is not a quiet one: the panel's own
  // line claims that nothing needs the reader, and a record nobody has
  // written anything about gives it no basis for that claim.
  it("says what a thin relationship HAS, and how far the reading reached", () => {
    show(VIEW, THIN);

    expect(screen.getByText(THIN.headline)).toBeTruthy();
    expect(screen.queryByText(en["today.quiet"])).toBeNull();
    expect(screen.getByText(en["contact.overview.coverage"])).toBeTruthy();
    expect(screen.queryByText(en["co.suggest.byline"])).toBeNull();
  });

  // A bar that names one task with no count reads as "that is everything".
  // A list of commitments may not say that.
  it("leads with one open task and counts the rest behind it", () => {
    const many = [1, 2, 3, 4, 5].map((n) => ({
      ...OPEN_TASK,
      id: `a-${n}`,
      subject: `Task ${n}`,
    }));
    show(
      { ...VIEW, next_steps: { data: many, page: { has_more: false } } },
      QUIET,
    );

    expect(screen.getByText("Task 1")).toBeTruthy();
    expect(
      screen.getAllByRole("button", { name: en["deal360.openTask"] }),
    ).toHaveLength(1);
    expect(
      screen.getByRole("button", {
        name: en["standing.next.more_other"].replace("{count}", "4"),
      }),
    ).toBeTruthy();
  });

  // No moment read is not a quiet record. The bar keeps its place and says
  // the reading is not shown rather than claiming nothing needs the reader.
  it("keeps an unread moment inside the panel as a reading it does not have", () => {
    show(VIEW);

    expect(screen.getByText(en["record.notShown"])).toBeTruthy();
    expect(screen.queryByText(en["today.quiet"])).toBeNull();
  });

  it("names the sources the day's work could not be read from", () => {
    show({ ...VIEW, sections_omitted: ["next_steps"] }, QUIET);

    expect(
      screen.getByText(
        en["today.withheld"].replace(
          "{sections}",
          en["today.source.nextSteps"],
        ),
      ),
    ).toBeTruthy();
  });
});

it("opens the actual task from the contact's attention list", async () => {
  const user = userEvent.setup();
  show(
    { ...VIEW, next_steps: { data: [OPEN_TASK], page: { has_more: false } } },
    QUIET,
  );
  await user.click(
    screen.getByRole("button", { name: en["deal360.openTask"] }),
  );
  expect(
    await screen.findByRole("dialog", { name: OPEN_TASK.subject }),
  ).toBeTruthy();
});

describe("Call on the next task", () => {
  const DIALABLE: Contact360 = {
    ...VIEW,
    contact: {
      ...VIEW.contact,
      phones: [
        {
          id: "ph-1",
          phone: "+49 30 1234567",
          phone_type: "mobile",
          is_primary: true,
          position: 0,
          ...CAPTURED,
        },
      ],
    },
  };
  const onTask = (sourceActivityId: string | null): Contact360 => ({
    ...DIALABLE,
    next_steps: {
      data: [
        {
          ...OPEN_TASK,
          source_activity_id: sourceActivityId,
          links: [{ entity_type: "contact", entity_id: "p-1" }],
        },
      ],
      page: { has_more: false },
    },
  });
  const dial = () => document.querySelector('a[href^="tel:"]');

  beforeEach(() =>
    installFetchStub({
      "GET /me": meRoute({ activity: ["read"] }),
      "GET /activities/call-1": () =>
        jsonResponse({
          id: "call-1",
          kind: "call",
          content_state: "available",
          occurred_at: "2026-08-09T09:00:00Z",
        }),
    }),
  );

  it("offers Call beside Open task when the task came from a call", async () => {
    show(onTask("call-1"), QUIET);

    const call = await screen.findByRole("link", {
      name: en["record.verb.call"],
    });
    expect(call.getAttribute("href")).toBe("tel:+49301234567");
    expect(
      screen.getByRole("button", { name: en["deal360.openTask"] }),
    ).toBeTruthy();
  });

  it("offers no Call on a task typed by hand", () => {
    show(onTask(null), QUIET);

    expect(screen.getByText(OPEN_TASK.subject)).toBeTruthy();
    expect(dial()).toBeNull();
  });

  // On this page the task is about this contact whatever else it links. The
  // task dialog, which has no page contact, gives a two-contact task no verbs.
  it("offers Call for this contact on a task that also links another", async () => {
    const view = onTask("call-1");
    const [task] = view.next_steps?.data ?? [];
    show(
      {
        ...view,
        next_steps: {
          data: task
            ? [
                {
                  ...task,
                  links: [
                    { entity_type: "contact", entity_id: "p-1" },
                    { entity_type: "contact", entity_id: "p-2" },
                  ],
                },
              ]
            : [],
          page: { has_more: false },
        },
      },
      QUIET,
    );

    const call = await screen.findByRole("link", {
      name: en["record.verb.call"],
    });
    expect(call.getAttribute("href")).toBe("tel:+49301234567");
  });
});

describe("Call on the move a call left owed", () => {
  const PHONE: ContactPhone = {
    id: "ph-1",
    phone: "+49 30 1234567",
    phone_type: "mobile",
    is_primary: true,
    position: 0,
    ...CAPTURED,
  };
  const CALLED: Activity = {
    ...OPEN_TASK,
    id: "a-7",
    subject: "Call Dana back on Monday",
    source_activity_id: "call-1",
    links: [{ entity_type: "contact", entity_id: "p-1" }],
  };
  const TYPED: Activity = { ...CALLED, id: "a-6", source_activity_id: null };
  const OWED: ContactMoment = {
    ...PROMISE,
    headline: "You owe them: Call Dana back on Monday",
    evidence: [{ type: "task", id: "a-7", label: "Call Dana back" }],
    recommended_action: {
      kind: "draft_reply",
      label: "Write to them",
      state: "available",
    },
  };
  const viewWith = (contact: Partial<Contact360["contact"]>): Contact360 => ({
    ...VIEW,
    contact: { ...VIEW.contact, phones: [PHONE], ...contact },
    next_steps: { data: [CALLED, TYPED], page: { has_more: false } },
  });
  const dial = () => document.querySelector('a[href^="tel:"]');

  beforeEach(() =>
    installFetchStub({
      "GET /me": meRoute({ activity: ["read"] }),
      "GET /activities/call-1": () =>
        jsonResponse({
          id: "call-1",
          kind: "call",
          content_state: "available",
          occurred_at: "2026-08-09T09:00:00Z",
        }),
    }),
  );

  it("offers Call beside the server's own action when the task came from a call", async () => {
    show(viewWith({}), OWED);

    const call = await screen.findByRole("link", {
      name: en["record.verb.call"],
    });
    expect(call.getAttribute("href")).toBe("tel:+49301234567");
    expect(screen.getByRole("button", { name: "Write to them" })).toBeTruthy();
  });

  const quiet: [
    string,
    Partial<ContactMoment>,
    Partial<Contact360["contact"]>,
  ][] = [
    [
      "a hand-typed task",
      { evidence: [{ type: "task", id: "a-6", label: "Typed" }] },
      {},
    ],
    [
      "a record that is not a task",
      { evidence: [{ type: "activity", id: "a-7", label: "Re: renewal" }] },
      {},
    ],
    [
      "a task the record does not list",
      { evidence: [{ type: "task", id: "a-5", label: "Gone" }] },
      {},
    ],
    ["an archived contact", {}, { archived_at: "2026-08-01T00:00:00Z" }],
  ];
  it.each(quiet)(
    "offers no Call on a move about %s",
    async (_, moment, contact) => {
      const client = show(viewWith(contact), { ...OWED, ...moment });

      // Every read has answered, so a missing Call is the decision, not a wait.
      await waitFor(() => expect(client.isFetching()).toBe(0));
      expect(
        screen.getByRole("button", { name: "Write to them" }),
      ).toBeTruthy();
      expect(dial()).toBeNull();
    },
  );

  // The promise, dated 12 days late and read out of a call; the evidence names
  // a hand-typed task 2 days late. Both the badge and Call follow the promise.
  const promised = (evidence: ContactMoment["evidence"]): ContactMoment => ({
    ...OWED,
    evidence,
    may_be_done: {
      promise_type: "task",
      promise_id: "a-7",
      email_activity_id: "e-1",
      wrote_at: "2026-08-12T10:00:00Z",
    },
  });
  const datedView = (): Contact360 => ({
    ...viewWith({}),
    next_steps: {
      data: [
        { ...CALLED, due_at: "2026-08-01T09:00:00Z" },
        { ...TYPED, due_at: "2026-08-11T09:00:00Z" },
      ],
      page: { has_more: false },
    },
  });
  const lateBy = (days: number) =>
    en["contact.standing.late_other"].replace("{count}", String(days));

  it("dates the promise and offers its Call when the evidence names another task", async () => {
    show(datedView(), promised([{ type: "task", id: "a-6", label: "Typed" }]));

    expect(
      await screen.findByRole("link", { name: en["record.verb.call"] }),
    ).toBeTruthy();
    expect(screen.getByText(lateBy(12))).toBeTruthy();
    expect(screen.queryByText(lateBy(2))).toBeNull();
  });

  it("offers Call on a move that rests on the promise alone", async () => {
    show(datedView(), promised([]));

    expect(
      await screen.findByRole("link", { name: en["record.verb.call"] }),
    ).toBeTruthy();
  });
});

describe("the face of the move's verb", () => {
  const FOLLOW_UP: ContactMoment = {
    ...QUIET,
    claim_key: "moment:gone_quiet",
    rule: "gone_quiet",
    headline: "No reply recorded for 233 days",
    recommended_action: {
      kind: "draft_reply",
      label: "Draft a follow-up",
      state: "available",
      destination: { surface: "composer", prefill: { intent: "follow_up" } },
    },
  };

  it("wears indigo where the press has the model draft the mail", () => {
    show(VIEW, FOLLOW_UP);

    const draft = screen.getByRole("button", { name: "Draft a follow-up" });
    expect(draft.classList.contains("btn-ai")).toBe(true);
  });

  it("keeps the accent where the press only opens a form", () => {
    show(VIEW, {
      ...FOLLOW_UP,
      recommended_action: {
        kind: "log_activity",
        label: "Log an interaction",
        state: "available",
        destination: { surface: "activity_log" },
      },
    });

    const log = screen.getByRole("button", { name: "Log an interaction" });
    expect(log.classList.contains("btn-ai")).toBe(false);
  });
});

it("shows a privacy duty in place of the quiet answer and opens its review action", async () => {
  const action = vi.fn();
  const privacy: ContactMoment = {
    ...QUIET,
    rule: "privacy_notice_due",
    headline: "Privacy notice owed",
    why_now: "An unresolved disclosure duty is recorded for this contact.",
    recommended_action: {
      kind: "open_record",
      label: "Review privacy duty",
      state: "available",
      destination: { surface: "privacy_notice", entity_id: "case-1" },
    },
  };
  const user = userEvent.setup();
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider>
        <ContactToday view={VIEW} moment={privacy} onAction={action} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  expect(screen.getByText("Privacy notice owed")).not.toBeNull();
  expect(
    screen.queryByRole("button", { name: en["standing.next.snooze"] }),
  ).toBeNull();
  expect(screen.queryByText(en["standing.next.none"])).toBeNull();
  await user.click(screen.getByRole("button", { name: "Review privacy duty" }));
  expect(action).toHaveBeenCalledWith(privacy.recommended_action);
});

it("keeps a quiet relationship readable when compliance work is outside the reader's role", () => {
  show({ ...VIEW, sections_omitted: ["privacy_notice_duties"] }, QUIET);
  expect(screen.getByText(en["standing.next.none"])).not.toBeNull();
  expect(screen.queryByText(/Not included: privacy duties/)).toBeNull();
});
