/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../../api/schema";
import { LocaleProvider } from "../../../i18n";
import { en } from "../../../i18n/en";
import { meFixture } from "../../../testing/mefixture";
import type { IntroRequest } from "../introrequests";
import { useNetworkReadings } from "./decision";

// What each reading says when the payload is SHORT of what it rests on.
//
// The hook is read directly rather than through the tab. These are the
// readings' own contract: a value is a non-empty string, and a detail is a
// receipt, never a substituted one. Through the page, the fixture would need
// a graph, an ask list and a map.

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

type Props = Parameters<typeof useNetworkReadings>[0];

function readings(props: Partial<Props> = {}) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
  return renderHook(
    () =>
      useNetworkReadings({
        routes: [],
        legacyVia: undefined,
        change: undefined,
        changeWithheld: false,
        open: undefined,
        ...props,
      }),
    { wrapper },
  ).result;
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
    origin: "asked",
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
    const result = readings({ change: changes.span });

    expect(result.current.progress.answer).toContain(
      `${en["contact.intro.change.quiet"]}.`,
    );
  });

  it("states a band move with no bands without naming two", () => {
    stubSession();
    const result = readings({ change: changes.bands });

    expect(result.current.progress.answer).toContain(
      `${en["contact.intro.change.warmed"]}.`,
    );
    expect(result.current.progress.answer).not.toContain("→");
  });

  // The span IS the receipt where it arrived, so the drop above is a real
  // branch rather than a reading that never carries one.
  it("states the span where the change carries it", () => {
    stubSession();
    const result = readings({ change: { ...changes.span, days: 62 } });

    expect(result.current.progress.answer).toContain(
      en["contact.intro.change.quietSub"].replace("{days}", "62"),
    );
  });
});

// A name the server sent as whitespace is a name it does not have. Letting it
// through reads as a reading which failed to load.
describe("a name the server sent blank is no name", () => {
  it("reads a blank legacy route name as no way in", () => {
    stubSession();
    const result = readings({ legacyVia: "   " });

    expect(result.current.wayIn.chip).toBe(en["contact.intro.stripNoRoutes"]);
    expect(result.current.wayIn.answer).toBe(en["contact.intro.stripNoPath"]);
  });

  it("still names a legacy route the server did name", () => {
    stubSession();
    const result = readings({ legacyVia: "Sofia Meier" });

    expect(result.current.wayIn.chip).toBe("Sofia Meier");
    expect(result.current.wayIn.answer).toContain(
      en["contact.intro.stripDirect"],
    );
  });
});

// Once the colleague has agreed, the move is the requester's. Where the
// payload leaves them unnamed, only the session can name them. It may do so
// only when the reader is that requester.
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
  const owner = en["contact.intro.handoffOwner"].replace(
    "{name}",
    meFixture({}).user.display_name,
  );

  it("leaves the owner off while the session is still being read", () => {
    stubSession("pending");
    const result = readings({ open: mine });

    expect(result.current.progress.chip).toBe(
      en["contact.intro.stateAccepted"],
    );
    expect(result.current.progress.answer).not.toContain("(");
  });

  it("names the reader once the session has answered", async () => {
    stubSession();
    const result = readings({ open: mine });

    await waitFor(() =>
      expect(result.current.progress.answer).toContain(owner),
    );
  });

  // The ask is somebody else's. The session can still be read, and it still
  // must not be the answer. The wait is a route the READER owns, read off the
  // same session query, so the absence is checked after the session landed.
  it("signs no other requester's ask with the reader's name", async () => {
    stubSession();
    const result = readings({ open: theirs, routes: [ownRoute] });
    await waitFor(() =>
      expect(result.current.wayIn.answer).toContain(
        en["contact.intro.stripWhoOwn"],
      ),
    );

    expect(result.current.progress.answer).not.toContain(
      meFixture({}).user.display_name,
    );
  });

  // A settled ask owes nobody anything.
  it("names nobody on an ask that has been answered for good", () => {
    stubSession();
    const result = readings({ open: ask({ status: "declined" }) });

    expect(result.current.progress.chip).toBe(
      en["contact.intro.stateDeclined"],
    );
    expect(result.current.progress.answer).not.toContain("(");
  });
});

describe("the ways in are counted, and the reader is one of them", () => {
  it("counts every route and names the reader's own in its own sentence", async () => {
    stubSession();
    const result = readings({ routes: [route, ownRoute] });

    await waitFor(() =>
      expect(result.current.wayIn.answer).toContain(
        en["contact.intro.stripWhoOwn"],
      ),
    );
    expect(result.current.wayIn.chip).toBe("2 routes");
  });
});

// An ask list that could not be read is not an empty one.
describe("the handoff over an unread ask list", () => {
  it("says it could not be read instead of not started", () => {
    stubSession();
    const result = readings({ openUnread: true });

    expect(result.current.progress.chip).toBe(
      en["contact.intro.handoffUnread"],
    );
  });

  it("says not started when the list was read and holds no ask", () => {
    stubSession();
    const result = readings();

    expect(result.current.progress.chip).toBe(
      en["contact.intro.handoffNotStarted"],
    );
  });
});
