/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { ContactToday } from "./contacttoday";
import { installFetchStub, jsonResponse, meRoute } from "./story-utils";

// THE DAY'S WORK GIVES ONE ANSWER, AND GIVES IT ONCE. The panel answers "what
// needs me" on every contact, so the quiet rung and the thin relationship are
// answers inside it rather than a reason to draw nothing; and the one thing
// the move is about is named once, rather than as the ask, the record it
// rests on and a chore underneath it.
//
// Assertions read the catalog rather than quoting English: the sentences here
// are product copy and a test pinning them fails on a rewrite that changed
// nothing about the behaviour.
//
// Mounted on the card rather than through the whole page: the page's own suite
// is at its length ceiling, and what is under test here is the panel's
// composition, which needs neither the header nor the rail.

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
  id: "a-9",
  kind: "task",
  subject: "Send the renewal quote",
  occurred_at: "2026-08-10T09:00:00Z",
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
        <ContactToday view={view} moment={moment} onAction={() => {}} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

describe("the move the panel leads with", () => {
  it("asks for THIS contact's move and gives the server's own reason", () => {
    show(VIEW, PROMISE);

    expect(screen.getByText(PROMISE.headline)).toBeTruthy();
    // The server's reason and no other: the facts in it are what a rep
    // judges, and the account brief reads the same moment the same way.
    expect(screen.getByText(PROMISE.why_now)).toBeTruthy();
    // The rule qualifies the byline: what the record was read against, beside
    // whose reading it is.
    expect(
      screen.getByText(en["contact.moment.rule.open_promise"]),
    ).toBeTruthy();
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
    // The subject is in the ask and nowhere else: no row of its own under the
    // move that already asks for it.
    expect(screen.queryByText(OPEN_TASK.subject)).toBeNull();
    // Every other commitment the record carries still stands.
    expect(screen.getByText(OTHER_TASK.subject)).toBeTruthy();
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

  // The quiet rung IS the answer to the panel's question, so it is answered
  // where the question is asked. Rendering nothing at all left a reader
  // unable to tell a quiet day from a pane that failed to load.
  it("answers a quiet record inside the panel", () => {
    show(VIEW, QUIET);

    expect(
      screen.getByRole("heading", { name: en["today.title"] }),
    ).toBeTruthy();
    expect(screen.getByText(en["today.quiet"])).toBeTruthy();
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

  // A truncated list with no count reads as "that is everything", which is
  // the one thing a list of commitments may not say.
  it("says how many open tasks the cut left out", () => {
    const many = [1, 2, 3, 4, 5].map((n) => ({
      ...OPEN_TASK,
      id: `a-${n}`,
      subject: `Task ${n}`,
    }));
    show(
      { ...VIEW, next_steps: { data: many, page: { has_more: false } } },
      QUIET,
    );

    expect(
      screen.getAllByRole("button", { name: en["deal360.openTask"] }),
    ).toHaveLength(3);
    expect(
      screen.getByText(en["co.suggest.more"].replace("{count}", "2")),
    ).toBeTruthy();
  });

  // No moment read is not a quiet record: the panel keeps its place and says
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
