/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { meFixture } from "../../app/mefixture";
import { LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import type { IntroRequest } from "../introrequests";
import { DecisionStrip } from "./decision";

// What each slot says when the payload is SHORT of what the slot rests on.
//
// The strip is mounted directly rather than through the tab: these are the
// card's own contract — a value is a non-empty string, a detail is a receipt
// and never a substituted one — and reading them through the page would mean
// the fixture had to satisfy a graph, an ask list and a map to prove anything
// about three cards.
//
// The tab's own wiring (which change reaches the strip, and whether the
// section was withheld) stays in index.test.tsx, where the 360 is.

type RouteCandidate = components["schemas"]["ContactGraphRouteCandidate"];
type RelationshipChange = components["schemas"]["ContactRelationshipChange"];

const CONTACT = "018f3a1b-0000-7000-8000-000000000010";
const SOFIA = "018f3a1b-0000-7000-8000-000000000021";

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

/**
 * The session the strip reads its own reader's name from, and a signal for
 * when it has answered.
 *
 * `pending` is a probe that never settles, which is the state the flicker
 * case is about — a promise that never resolves rather than a slow one,
 * because a duration would make the case a race with the scheduler.
 *
 * `answered` is what a case asserting the owner line is ABSENT has to wait on.
 * The value renders without the session, so an absence checked before the
 * probe lands passes against a strip that has not consulted it yet — which is
 * exactly the reading the case is about.
 */
function stubSession(session: "answered" | "pending" = "answered") {
  let landed: () => void = () => {};
  const answered = new Promise<void>((resolve) => {
    landed = resolve;
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => {
      if (session === "pending") {
        return new Promise<Response>(() => {});
      }
      landed();
      return new Response(JSON.stringify(meFixture({ roles: ["rep"] })), {
        status: 200,
        headers: { "content-type": "application/json" },
      });
    }),
  );
  return { answered };
}

function renderStrip(
  props: Partial<Parameters<typeof DecisionStrip>[0]> = {},
): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <DecisionStrip
          routes={[]}
          legacyVia={undefined}
          change={undefined}
          changeWithheld={false}
          open={undefined}
          {...props}
        />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

// The card a reading is drawn in, found by the value it drew.
function cardOf(value: string): HTMLElement {
  const card = screen
    .getByText(value, { selector: ".stat-card-value" })
    .closest(".stat-card");
  if (!(card instanceof HTMLElement)) {
    throw new Error(`"${value}" is not drawn in a card`);
  }
  return card;
}

const route: RouteCandidate = {
  route_id: `direct:${SOFIA}`,
  route_type: "direct",
  via_user_id: SOFIA,
  via_display_name: "Sofia Meier",
  strength_bucket: "strong",
  evidence: { interactions_90d: 6, two_way: true },
  availability: "available",
};

// The route the READER is on, which `useOwnRoute` recognises off the same
// session query the handoff reads its own name from.
const ownRoute: RouteCandidate = {
  ...route,
  via_user_id: meFixture({}).user.id,
};

function ask(over: Partial<IntroRequest> = {}): IntroRequest {
  return {
    id: "018f3a1b-0000-7000-8000-0000000000a1",
    contact_id: CONTACT,
    requester_user_id: "018f3a1b-0000-7000-8000-0000000000a9",
    introducer_user_id: SOFIA,
    introducer_display_name: "Sofia Meier",
    route_type: "direct",
    status: "requested",
    internal_reason: "Renewal conversation",
    name_drop_allowed: false,
    note_generated_by: "human",
    note_ai_generated: false,
    fallback_policy: "none",
    requested_at: "2026-09-03T09:00:00Z",
    due_at: "2026-09-08T09:00:00Z",
    version: 1,
    ...over,
  };
}

// A receipt standing on a substituted value is worse than none: "After 0 d
// quiet" and "no contact → no contact" both read as figures the server sent.
// The verdict is still true, so the word stays and the line under it goes.
describe("the latest change states no receipt it does not have", () => {
  const changes: Record<string, RelationshipChange> = {
    span: { kind: "went_quiet", at: "2026-08-20T09:00:00Z" },
    bands: { kind: "warmed", at: "2026-08-25T09:00:00Z" },
  };

  it("states a change with no span without inventing one", () => {
    stubSession();
    renderStrip({ change: changes.span });

    const card = cardOf(en["contact.intro.change.quiet"]);
    expect(card.querySelector(".stat-card-detail")).toBeNull();
  });

  it("states a band move with no bands without naming two", () => {
    stubSession();
    renderStrip({ change: changes.bands });

    const card = cardOf(en["contact.intro.change.warmed"]);
    expect(card.querySelector(".stat-card-detail")).toBeNull();
    expect(screen.queryByText(/→/)).toBeNull();
  });

  // The span IS the receipt where it arrived, so the drop above is a real
  // branch rather than a slot that never carries one.
  it("states the span where the change carries it", () => {
    stubSession();
    renderStrip({ change: { ...changes.span, days: 62 } });

    const card = cardOf(en["contact.intro.change.quiet"]);
    expect(card.textContent).toContain(
      en["contact.intro.change.quietSub"].replace("{days}", "62"),
    );
  });
});

// A StatCard value is a non-empty string by contract. A name the server sent
// as whitespace is a name it does not have, and letting it through draws a
// slot that reads as a reading which failed to load.
describe("a name the server sent blank is no name", () => {
  it("reads a blank legacy route name as no way in", () => {
    stubSession();
    renderStrip({ legacyVia: "   " });

    expect(screen.getByText(en["contact.intro.stripNoRoutes"])).toBeTruthy();
    expect(screen.getByText(en["contact.intro.stripNoPath"])).toBeTruthy();
  });

  it("still names a legacy route the server did name", () => {
    stubSession();
    renderStrip({ legacyVia: "Sofia Meier" });

    expect(screen.getByText("Sofia Meier")).toBeTruthy();
    expect(screen.getByText(en["contact.intro.stripDirect"])).toBeTruthy();
  });
});

// Once the colleague has agreed the move is the REQUESTER's, and where the
// payload leaves them unnamed the session is the only thing that can name
// them — which it may do only when the reader IS that requester. Two ways to
// get this wrong: standing a colleague in while the read is in flight made the
// slot say someone owed the move and then replace them with another, and
// signing an ask from a different requester with the reader's own name states
// a fact about the record that nothing supports.
describe("the handoff names nobody it cannot name yet", () => {
  const mine = ask({
    status: "accepted",
    requester_user_id: meFixture({}).user.id,
    requester_display_name: undefined,
    decided_at: "2026-09-04T09:00:00Z",
  });
  const theirs = ask({
    ...mine,
    requester_user_id: "018f3a1b-0000-7000-8000-0000000000a9",
  });

  it("leaves the owner line off while the session is still being read", () => {
    stubSession("pending");
    renderStrip({ open: mine });

    const card = cardOf(en["contact.intro.stateAccepted"]);
    expect(card.querySelector(".stat-card-detail")).toBeNull();
    expect(screen.queryByText(en["contact.intro.ownerColleague"])).toBeNull();
  });

  it("names the reader once the session has answered", async () => {
    stubSession();
    renderStrip({ open: mine });

    expect(
      await screen.findByText(
        en["contact.intro.handoffOwner"].replace(
          "{name}",
          meFixture({}).user.display_name,
        ),
      ),
    ).toBeTruthy();
  });

  // The ask is somebody else's. The session can still be read, and it still
  // must not be the answer: an unnamed requester stays unnamed.
  //
  // The wait is a route the READER owns, drawn from the same session query, so
  // the absence below is checked against a strip that has consulted the
  // session rather than one that has not reached it yet.
  it("signs no other requester's ask with the reader's name", async () => {
    stubSession();
    renderStrip({ open: theirs, routes: [ownRoute] });
    await screen.findByText(en["contact.intro.stripWhoOwn"]);

    const card = cardOf(en["contact.intro.stateAccepted"]);
    expect(card.querySelector(".stat-card-detail")).toBeNull();
    expect(
      screen.queryByText(new RegExp(meFixture({}).user.display_name)),
    ).toBeNull();
  });

  // A settled ask owes nobody anything, so naming an owner there would point
  // at a colleague who has already done their part.
  it("names nobody on an ask that has been answered for good", () => {
    stubSession();
    renderStrip({ open: ask({ status: "declined" }) });

    const card = cardOf(en["contact.intro.stateDeclined"]);
    expect(card.querySelector(".stat-card-detail")).toBeNull();
  });
});

// The fold is the PLATE's — `.stat-strip:has(.stat-card-narrow-row)` restyles
// the whole row — so a strip where only some cards declared it would draw a
// bordered box among a column of borderless rows.
describe("every slot on the strip declares the narrow shape", () => {
  it("leaves no card behind when the row folds", () => {
    stubSession();
    renderStrip({ change: { kind: "warmed", at: "2026-08-25T09:00:00Z" } });

    expect(document.querySelectorAll(".stat-card-narrow-row").length).toBe(3);
  });
});

describe("the ways in are counted, and the reader is one of them", () => {
  it("counts every route and names the reader's own on its own line", async () => {
    stubSession();
    renderStrip({ routes: [route, ownRoute] });

    expect(
      await screen.findByText(en["contact.intro.stripWhoOwn"]),
    ).toBeTruthy();
    expect(cardOf("2")).toBeTruthy();
  });
});
