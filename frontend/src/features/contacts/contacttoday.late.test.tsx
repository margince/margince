/** @vitest-environment happy-dom */
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { ViewerZoneProvider } from "../../lib/viewerzone";
import { installFetchStub, meRoute } from "../../testing/story-utils";
import { ContactToday } from "./contacttoday";

type Contact360 = components["schemas"]["Contact360"];
type ContactMoment = components["schemas"]["ContactMoment"];

beforeEach(() => installFetchStub({ "GET /me": meRoute({}) }));
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// Due 23:00 Berlin on the 11th, read at 10:00 Berlin on the 13th: 35 hours,
// so the server's elapsed count says one day while the calendar says two.
const DUE = "2026-08-11T21:00:00Z";
const AS_OF = "2026-08-13T08:00:00Z";
const CAPTURED = {
  source: "manual",
  captured_by: "human:u-1",
  created_at: "2026-06-01T08:00:00Z",
  updated_at: "2026-08-01T08:00:00Z",
} as const;

function owed(evidence: ContactMoment["evidence"]): ContactMoment {
  return {
    claim_key: "moment:overdue_promise",
    evidence_fingerprint: "late",
    rule: "overdue_promise",
    headline: "You owe them: send the contract",
    why_now: "Due one day ago and still open.",
    confidence: "observed_fact",
    evidence,
    recommended_action: {
      kind: "complete_task",
      label: "Open it",
      state: "available",
    },
  };
}

const VIEW: Contact360 = {
  as_of: AS_OF,
  contact: { id: "p-1", full_name: "Dana Buyer", ...CAPTURED },
  sections_omitted: [],
  next_steps: {
    data: [
      {
        offers_email: false,
        id: "a-1",
        kind: "task",
        subject: "Send the contract",
        occurred_at: "2026-08-10T09:00:00Z",
        occurred_precision: "instant",
        is_done: false,
        due_at: DUE,
        ...CAPTURED,
      },
    ],
    page: { has_more: false },
  },
  claims: [
    {
      id: "c-1",
      kind: "commitment_ours",
      body: "Send the contract",
      source_activity_id: "e-1",
      source_quote: "We will send the contract by Tuesday.",
      occurred_precision: "instant",
      status: "open",
      needs_review: false,
      due_at: DUE,
    },
  ],
};

function show(moment: ContactMoment) {
  render(
    <QueryClientProvider client={new QueryClient()}>
      <LocaleProvider initial="en">
        <ViewerZoneProvider zone="Europe/Berlin">
          <ContactToday
            view={VIEW}
            moment={moment}
            onAction={() => {}}
            onOpenTasks={() => {}}
          />
        </ViewerZoneProvider>
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const badge = en["co.next.daysLate_other"].replace("{days}", "2");

describe("a late promise on the contact's Next bar", () => {
  it("counts a task's days the same in its badge and its sentence", () => {
    show(owed([{ type: "task", id: "a-1", label: "Send the contract" }]));
    expect(screen.getByText(badge)).toBeTruthy();
    expect(
      screen.getByText(
        en["promise.late.sentence_other"].replace("{days}", "2"),
      ),
    ).toBeTruthy();
    expect(screen.queryByText("Due one day ago and still open.")).toBeNull();
  });

  it("counts a claim's days the same in its badge and its sentence", () => {
    // A claim moment cites the message it was read from; the promise it may
    // have settled names the claim itself.
    show({
      ...owed([{ type: "activity", id: "e-1", label: "Re: contract" }]),
      may_be_done: {
        promise_type: "claim",
        promise_id: "c-1",
        email_activity_id: "e-1",
        wrote_at: "2026-08-12T10:00:00Z",
      },
    });
    expect(screen.getByText(badge)).toBeTruthy();
    expect(
      // The sentence shares its line with the source it is based on.
      screen.getByText(
        en["promise.claim.sentence_other"].replace("{days}", "2"),
        { exact: false },
      ),
    ).toBeTruthy();
  });
});
