import { describe, expect, it } from "vitest";
import {
  dirtyKeys,
  displayOn,
  initialDraft,
  type PurposeView,
  toChoices,
} from "./preferences.logic";

const purposes: PurposeView[] = [
  {
    key: "transactional",
    label: "Deal & service messages",
    state: "granted",
    locked: true,
    // A locked purpose cannot be offered a grant from this surface, which is
    // what can_opt_in says; the choice is what the recipient decided.
    choice: "opted_in",
    can_opt_in: false,
    grant_needs_confirmation: false,
    choice: "opted_in",
    can_opt_in: false,
  },
  {
    key: "marketing_email",
    label: "Product updates",
    state: "granted",
    locked: false,
    choice: "opted_in",
    can_opt_in: true,
    grant_needs_confirmation: false,
    choice: "opted_in",
    can_opt_in: true,
  },
  {
    key: "events",
    label: "Events",
    state: "withdrawn",
    locked: false,
    choice: "opted_out",
    can_opt_in: true,
    grant_needs_confirmation: false,
    choice: "opted_out",
    can_opt_in: true,
  },
  {
    key: "research",
    label: "Surveys",
    state: "unknown",
    locked: false,
    // "unknown" reads differently either side of the consent line, which is
    // why the server derives `choice` at all: nobody has objected here.
    choice: "no_objection",
    can_opt_in: true,
    grant_needs_confirmation: false,
    choice: "opted_out",
    can_opt_in: true,
  },
];

// A purpose the server will only grant through a confirmation round-trip.
// Withdrawing it here is honoured; granting it is refused, so the two
// directions are asserted apart.
const doiPurposes: PurposeView[] = [
  {
    key: "doi_newsletter",
    label: "Newsletter",
    state: "withdrawn",
    locked: false,
    choice: "opted_out",
    // A purpose needing a confirmation round-trip cannot be granted from
    // here either, for the same reason a locked one cannot.
    can_opt_in: false,
    grant_needs_confirmation: true,
    choice: "opted_out",
    can_opt_in: false,
  },
  {
    key: "doi_granted",
    label: "Already subscribed",
    state: "granted",
    locked: false,
    choice: "opted_in",
    can_opt_in: false,
    grant_needs_confirmation: true,
    choice: "opted_in",
    can_opt_in: false,
  },
];

describe("display state", () => {
  // Default-deny: no record and a withdrawal both mean "we may not send".
  it("reads only an explicit grant as subscribed", () => {
    expect(displayOn("granted")).toBe(true);
    expect(displayOn("withdrawn")).toBe(false);
    expect(displayOn("unknown")).toBe(false);
  });
});

describe("draft diffing", () => {
  it("starts clean", () => {
    const draft = initialDraft(purposes);
    expect(draft).toEqual({
      transactional: true,
      marketing_email: true,
      events: false,
      research: false,
    });
    expect(dirtyKeys(purposes, draft)).toEqual([]);
  });

  it("reports only what the subject actually moved", () => {
    const draft = {
      ...initialDraft(purposes),
      marketing_email: false,
      events: true,
    };
    expect(dirtyKeys(purposes, draft)).toEqual(["marketing_email", "events"]);
  });

  it("never reports a locked purpose, even if a draft claims it moved", () => {
    const draft = { ...initialDraft(purposes), transactional: false };
    expect(dirtyKeys(purposes, draft)).toEqual([]);
  });
});

describe("choice building", () => {
  const wordingOf = (key: string) => `"Send me ${key}."`;

  // The load-bearing rule: an untouched purpose is never submitted. A choice
  // writes an append-only proof row, so submitting one the subject didn't
  // make would fabricate consent evidence.
  it("submits only changed purposes, with the wording shown", () => {
    const draft = {
      ...initialDraft(purposes),
      marketing_email: false,
      events: true,
    };
    expect(toChoices(purposes, draft, wordingOf)).toEqual([
      {
        purpose_key: "marketing_email",
        state: "withdrawn",
        wording: '"Send me marketing_email."',
      },
      { purpose_key: "events", state: "granted", wording: '"Send me events."' },
    ]);
  });

  it("submits nothing when nothing moved", () => {
    expect(toChoices(purposes, initialDraft(purposes), wordingOf)).toEqual([]);
  });
});

describe("a purpose needing a confirmation round-trip", () => {
  // The defect this holds: the page offered the switch, the person turned it
  // on, and the save came back 422 with nothing recorded.
  it("never submits a grant the server refuses", () => {
    const draft = { doi_newsletter: true, doi_granted: true };
    expect(dirtyKeys(doiPurposes, draft)).toEqual([]);
  });

  // And the half that must keep working: an unsubscribe is honoured, so
  // filtering the whole purpose would strip somebody's opt-out.
  it("still submits a withdrawal", () => {
    const draft = { doi_newsletter: false, doi_granted: false };
    expect(dirtyKeys(doiPurposes, draft)).toEqual(["doi_granted"]);
  });
});
