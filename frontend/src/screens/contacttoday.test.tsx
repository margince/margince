/** @vitest-environment jsdom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { ContactToday } from "./contacttoday";

// THE DAY'S WORK GIVES ONE ANSWER. The quiet rung is the answer a reader came
// for on a record with nothing pending, and a contradiction on one that has
// something — so which of the two it is depends on the rest of the list, and
// that is what these cases hold.
//
// Mounted on the card rather than through the whole page: the page's own suite
// is at its length ceiling, and what is under test here is the panel's
// composition, which needs neither the header nor the rail.

afterEach(cleanup);

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
  why_now:
    "No meeting is close, nothing is owed, and nobody is waiting on a reply.",
  confidence: "observed_fact",
  evidence: [],
  recommended_action: {
    kind: "log_activity",
    label: "Log an interaction",
    state: "available",
    destination: { surface: "activity_log" },
  },
};

const OPEN_TASK = {
  id: "a-9",
  kind: "task",
  subject: "Send the renewal quote",
  occurred_at: "2026-08-10T09:00:00Z",
  is_done: false,
  ...CAPTURED,
} as const;

function show(view: Contact360, moment: ContactMoment) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <ContactToday
          name="Dana Buyer"
          view={view}
          moment={moment}
          onAction={() => {}}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

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

    expect(screen.getByText("Send the renewal quote")).toBeTruthy();
    expect(screen.queryByText("Nothing needs you today")).toBeNull();
  });

  // Dropped only where it would contradict. With nothing else in the list the
  // quiet card IS the answer, and it keeps the verb the ladder named on it.
  it("keeps the quiet card when it is the whole answer", () => {
    show(VIEW, QUIET);

    expect(screen.getByText("Nothing needs you today")).toBeTruthy();
    expect(
      screen.getByRole("button", { name: "Log an interaction" }),
    ).toBeTruthy();
  });
});
