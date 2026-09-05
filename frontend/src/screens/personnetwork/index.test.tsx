/** @vitest-environment jsdom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { components } from "../../api/schema";
import { type GrantSpec, meFixture } from "../../app/mefixture";
import { type Locale, LocaleProvider } from "../../i18n";
import { en } from "../../i18n/en";
import { PersonNetworkTab } from "./index";

// What the tab says when it has LESS to say than the full case.
//
// Both cases here were found by looking at the rendered stories rather than by
// reading the code, and both look fine in the source: the page said "nobody
// reaches this contact" twice in two weights, and drew a card headed "Pick the
// one you can actually use" above an empty list. Nothing threw, every element
// existed, and a DOM query would have passed.

type PersonGraph = components["schemas"]["PersonGraph"];
type IntroRequest = components["schemas"]["IntroRequest"];
type Moments = Pick<
  components["schemas"]["Person360"],
  "relationship_changes" | "sections_omitted"
>;
type Route = NonNullable<PersonGraph["routes"]>[number];

const PERSON = "018f3a1b-0000-7000-8000-000000000010";

// The strings come from the catalogue rather than being typed here, so a
// rewording is a copy change and not a test failure. What these tests are about
// is how MANY times the page says a thing and whether a card is drawn at all —
// neither of which should depend on the words chosen.
const NO_ROUTE = en["person.graph.noRoute"];
const WAYS_IN = en["person.intro.routesTitle"];

function jsonResponse(body: unknown): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "content-type": "application/json" },
  });
}

/**
 * The seat the tab is read as, and a signal for when it has answered.
 *
 * `/me` answers the grant map every capability hook consults, and the default
 * fixture grants nothing — so a case about a write control has to say which
 * seat is looking. `seatAnswered` is what a case asserting a control is ABSENT
 * waits on: every capability hook reads false while the snapshot is in flight,
 * so an ungranted seat and an unanswered one draw the identical page, and a
 * query run before the answer lands would pass against a page that has not yet
 * consulted the grants at all.
 */
function stub(
  graph: PersonGraph,
  allow: GrantSpec = {},
  asks: readonly IntroRequest[] = [],
) {
  let answered: () => void = () => {};
  const seatAnswered = new Promise<void>((resolve) => {
    answered = resolve;
  });
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL) => {
      const url = String(input instanceof Request ? input.url : input);
      if (url.includes("/graph")) {
        return jsonResponse(graph);
      }
      if (url.includes("/intro-requests")) {
        return jsonResponse({ data: asks });
      }
      if (url.includes("/me")) {
        answered();
        return jsonResponse(meFixture({ roles: ["rep"], allow }));
      }
      return jsonResponse({ data: [] });
    }),
  );
  return { seatAnswered };
}

function renderTab(
  graph: PersonGraph,
  locale: Locale = "en",
  allow: GrantSpec = {},
  // The two reads beside the graph: the asks in flight, and the 360's
  // moments. Absent by default, which is what the older contacts screen
  // hands the tab.
  beside: Readonly<{ asks?: readonly IntroRequest[]; view?: Moments }> = {},
) {
  const { seatAnswered } = stub(graph, allow, beside.asks);
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial={locale}>
        <PersonNetworkTab personId={PERSON} view={beside.view} />
      </LocaleProvider>
    </QueryClientProvider>,
  );
  return { seatAnswered };
}

const anchor: PersonGraph["nodes"][number] = {
  id: `person:${PERSON}`,
  type: "contact",
  group: "anchor",
  label: "Dana Buyer",
  person_id: PERSON,
};

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("a contact nobody here reaches", () => {
  it("says so once, not twice", async () => {
    renderTab({
      person_id: PERSON,
      nodes: [anchor],
      edges: [],
      routes: [],
      groups_omitted: [],
    });

    // The strip is this page's answer-first surface, so the sentence belongs
    // there. A paragraph under it repeating the same words made one finding
    // read as two.
    await waitFor(() => {
      expect(screen.getAllByText(NO_ROUTE).length).toBe(1);
    });
  });

  it("draws no card offering a choice of nothing", async () => {
    renderTab({
      person_id: PERSON,
      nodes: [anchor],
      edges: [],
      routes: [],
      groups_omitted: [],
    });

    await screen.findByText(NO_ROUTE);
    // "Ways in — best first, pick the one you can actually use" over an empty
    // list. The card's own comment says a heading with nothing under it is
    // worse than no card, and the condition that drew it disagreed.
    expect(screen.queryByText(WAYS_IN)).toBeNull();
  });
});

// A server that predates the candidate list answers with the singular `route`
// and no `routes`. That payload DOES have a way in, and the card is the only
// thing that can draw it — so an empty `routes` alone must not suppress it.
describe("a legacy payload with one route", () => {
  it("still draws the card that can show it", async () => {
    renderTab({
      person_id: PERSON,
      nodes: [
        anchor,
        {
          id: "user:018f3a1b-0000-7000-8000-000000000021",
          type: "colleague",
          group: "direct",
          label: "Sofia Meier",
          user_id: "018f3a1b-0000-7000-8000-000000000021",
        },
      ],
      edges: [
        {
          from: "user:018f3a1b-0000-7000-8000-000000000021",
          to: `person:${PERSON}`,
          strength_bucket: "strong",
          interactions_90d: 14,
        },
      ],
      // The singular route's own shape, which is NOT the candidate's: it
      // carries an English `why` sentence the server wrote, and no route_type.
      route: {
        via_user_id: "018f3a1b-0000-7000-8000-000000000021",
        via_display_name: "Sofia Meier",
        why: "Sofia has corresponded with them 14 times in 90 days.",
      },
      groups_omitted: [],
    });

    // findByText throws when absent, which IS the assertion.
    await screen.findByText(WAYS_IN);
    // And it does NOT claim nobody reaches them, which is the contradiction
    // the old condition was written to avoid.
    expect(screen.queryByText(NO_ROUTE)).toBeNull();
  });
});

// The lead panel and the alternatives list draw the same route, and they must
// call a blocked state the same thing.
//
// These three states were unreachable until the server learned to report them,
// and in that time two copies of the label switch drifted: one said
// "Unavailable" where the other said "Not available". Nothing failed, because
// nothing rendered. This is what fails now.
describe("a route that cannot be asked for", () => {
  const taken: NonNullable<PersonGraph["routes"]>[number] = {
    route_id: "direct:1",
    route_type: "direct",
    via_user_id: "018f3a1b-0000-7000-8000-0000000000a1",
    via_display_name: "Lena Fischer",
    strength_bucket: "strong",
    evidence: { interactions_90d: 12, two_way: true },
    // `unavailable` on purpose: it is the state whose two copies had actually
    // drifted ("Unavailable" against "Not available"), so a test built on it
    // fails when a second spelling comes back. A state whose two old spellings
    // happened to match would pass against the drift it exists to catch.
    availability: "unavailable",
  };

  it("is named the same way wherever it is drawn", async () => {
    renderTab({
      person_id: PERSON,
      nodes: [anchor],
      edges: [],
      // Two routes, so the lead panel draws the first and the alternatives
      // card draws the second: one label switch has to serve both.
      routes: [taken, { ...taken, route_id: "direct:2" }],
      groups_omitted: [],
    });

    const label = en["person.intro.unavailable"];
    await waitFor(() => {
      expect(screen.getAllByText(label).length).toBe(2);
    });
  });

  it("offers no button that would answer 409", async () => {
    renderTab({
      person_id: PERSON,
      nodes: [anchor],
      edges: [],
      routes: [taken],
      groups_omitted: [],
    });

    await screen.findByText(en["person.intro.unavailable"]);
    expect(
      screen.queryByRole("button", {
        name: en["person.intro.askFirstName"].replace(
          "{name}",
          taken.via_display_name,
        ),
      }),
    ).toBeNull();
  });
});

// The strip's first slot counts the ways in rather than naming the best one,
// because the verdict panel above it already names the lead. What it owes the
// reader is the mix: how many colleagues, and how many of them get there only
// through somebody at the account.
describe("several ways in", () => {
  const direct: NonNullable<PersonGraph["routes"]>[number] = {
    route_id: "direct:1",
    route_type: "direct",
    via_user_id: "018f3a1b-0000-7000-8000-0000000000a1",
    via_display_name: "Sofia Meier",
    strength_bucket: "strong",
    evidence: { interactions_90d: 6, two_way: true },
    availability: "available",
  };
  const viaContact: NonNullable<PersonGraph["routes"]>[number] = {
    ...direct,
    route_id: "through:1",
    route_type: "through_contact",
    via_user_id: "018f3a1b-0000-7000-8000-0000000000a2",
    via_display_name: "Lena Hoff",
    through_person_id: "018f3a1b-0000-7000-8000-0000000000b1",
    through_display_name: "Philipp Königs",
    strength_bucket: "moderate",
  };

  it("counts the colleagues and says how many go through a contact", async () => {
    renderTab({
      person_id: PERSON,
      nodes: [anchor],
      edges: [],
      routes: [
        direct,
        viaContact,
        { ...direct, route_id: "direct:2", via_display_name: "Martin Weber" },
      ],
      groups_omitted: [],
    });

    await screen.findByText(
      en["person.intro.stripWhoCount_other"].replace("{count}", "3"),
    );
    expect(
      screen.getByText(
        en["person.intro.stripWhoMix"]
          .replace("{direct}", "2")
          .replace("{indirect}", "1"),
      ),
    ).toBeTruthy();
    // The lead is the verdict panel's; the card under it lists the OTHER two,
    // ranked as the second and third way in rather than as a new list of one.
    expect(screen.getByText(en["person.intro.otherRoutesTitle"])).toBeTruthy();
    expect(screen.queryByText(WAYS_IN)).toBeNull();
    const ranks = Array.from(document.querySelectorAll(".pn-route-rank")).map(
      (rank) => rank.textContent,
    );
    expect(ranks).toEqual(["2", "3"]);
    // The strength meter carries its bucket as a word, so a reader who
    // cannot see three bars still gets the fact.
    expect(screen.getAllByText(en["person.band.moderate"]).length).toBe(1);
  });

  it("opens the ask drawer from an alternative's own button", async () => {
    const user = userEvent.setup();
    renderTab({
      person_id: PERSON,
      nodes: [anchor],
      edges: [],
      routes: [direct, viaContact],
      groups_omitted: [],
    });

    await user.click(
      await screen.findByRole("button", {
        name: en["person.intro.askFirstName"].replace("{name}", "Lena Hoff"),
      }),
    );
    expect(
      await screen.findByText(
        en["person.intro.askTitle"].replace("{name}", anchor.label),
      ),
    ).toBeTruthy();
  });
});

// The verdict panel states the move in one sentence and puts the figures it
// was written from beside it. The sentence has three shapes, and the plate
// has to be honest about what it has: a split only when the server split the
// count, receipts only when the route carries them.
describe("the verdict and its evidence", () => {
  const sofia: Route = {
    route_id: "direct:sofia",
    route_type: "direct",
    via_user_id: "018f3a1b-0000-7000-8000-0000000000a1",
    via_display_name: "Sofia Meier",
    strength_bucket: "strong",
    evidence: {
      interactions_90d: 6,
      inbound_90d: 4,
      outbound_90d: 2,
      two_way: true,
      days_since_last: 1,
    },
    receipts: [
      {
        activity_id: "018f3a1b-0000-7000-8000-0000000000e1",
        subject: "Re: Q4 rollout",
        occurred_at: "2026-08-28T09:00:00Z",
        kind: "email",
      },
      {
        activity_id: "018f3a1b-0000-7000-8000-0000000000e2",
        subject: "Northgate pilot",
        occurred_at: "2026-08-21T14:30:00Z",
        kind: "meeting",
      },
    ],
    availability: "available",
  };

  it("cites the receipts and splits the count by who sent it", async () => {
    const user = userEvent.setup();
    renderTab({
      person_id: PERSON,
      nodes: [anchor],
      edges: [],
      routes: [sofia],
      groups_omitted: [],
    });

    await screen.findByText(
      en["person.intro.verdictDirect"].replace("{name}", "Sofia Meier"),
    );
    expect(screen.getByText(en["person.intro.lastYesterday"])).toBeTruthy();
    expect(
      screen.getByText(
        en["person.intro.evidenceFrom"]
          .replace("{count}", "4")
          .replace("{name}", anchor.label),
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(
        en["person.intro.evidenceFrom"]
          .replace("{count}", "2")
          .replace("{name}", "Sofia Meier"),
      ),
    ).toBeTruthy();
    expect(
      screen.getByText(en["person.intro.factReceipts"].replace("{count}", "2")),
    ).toBeTruthy();
    // A mail is cited as a mail; a meeting keeps the plain line, so the
    // reader is not told it was a message.
    expect(screen.getByText("Re: Q4 rollout")).toBeTruthy();
    expect(screen.getByText(/Northgate pilot ·/)).toBeTruthy();

    await user.click(
      screen.getByRole("button", {
        name: en["person.intro.askFirstName"].replace("{name}", "Sofia Meier"),
      }),
    );
    expect(
      await screen.findByText(
        en["person.intro.askTitle"].replace("{name}", anchor.label),
      ),
    ).toBeTruthy();
  });

  it("names the colleague a route goes through, and says the counts are pooled", async () => {
    renderTab({
      person_id: PERSON,
      nodes: [anchor],
      edges: [],
      routes: [
        {
          ...sofia,
          route_id: "through:lena",
          route_type: "through_contact",
          via_display_name: "Lena Hoff",
          through_person_id: "018f3a1b-0000-7000-8000-0000000000b1",
          through_display_name: "Philipp Königs",
          evidence: { interactions_90d: 5, two_way: true, days_since_last: 6 },
          receipts: undefined,
        },
      ],
      groups_omitted: [],
    });

    await screen.findByText(
      en["person.intro.verdictVia"]
        .replace("{name}", "Lena Hoff")
        .replace("{through}", "Philipp Königs"),
    );
    expect(
      screen.getByText(en["person.intro.lastDays"].replace("{days}", "6")),
    ).toBeTruthy();
    expect(screen.getByText(en["person.graph.countsOnly"])).toBeTruthy();
    // No split was served, so no bar claims one.
    expect(document.querySelector(".pn-split")).toBeNull();
  });

  it("does not call unanswered sends a correspondence", async () => {
    renderTab({
      person_id: PERSON,
      nodes: [anchor],
      edges: [],
      routes: [
        {
          ...sofia,
          route_id: "direct:martin",
          via_display_name: "Martin Weber",
          strength_bucket: "weak",
          evidence: {
            interactions_90d: 3,
            inbound_90d: 0,
            outbound_90d: 3,
            two_way: false,
            days_since_last: null,
          },
          receipts: [],
        },
      ],
      groups_omitted: [],
    });

    await screen.findByText(
      en["person.intro.verdictOneSided"].replace("{name}", "Martin Weber"),
    );
    expect(screen.getByText(en["person.intro.factOneSided"])).toBeTruthy();
    expect(screen.getByText(en["person.intro.lastNever"])).toBeTruthy();
  });
});

// The handoff names who owes the next move under the steps, and the due date
// belongs only to the step that has one: the colleague's answer.
describe("the handoff with an ask open", () => {
  const ask: IntroRequest = {
    id: "018f3a1b-0000-7000-8000-0000000000c1",
    person_id: PERSON,
    requester_user_id: "018f3a1b-0000-7000-8000-0000000000a9",
    requester_display_name: "Demo Admin",
    introducer_user_id: "018f3a1b-0000-7000-8000-0000000000a1",
    introducer_display_name: "Sofia Meier",
    route_type: "direct",
    internal_reason: "Renewal conversation",
    note_generated_by: "human",
    note_ai_generated: false,
    name_drop_allowed: false,
    fallback_policy: "none",
    status: "requested",
    requested_at: "2026-09-03T09:00:00Z",
    due_at: "2026-09-08T09:00:00Z",
    version: 1,
  };
  const graph: PersonGraph = {
    person_id: PERSON,
    nodes: [anchor],
    edges: [],
    routes: [],
    groups_omitted: [],
  };

  it("says the colleague owes an answer, and by when", async () => {
    renderTab(graph, "en", {}, { asks: [ask] });

    // Once on the strip's handoff slot, once under the steps: the same
    // words, from the same reading of the ask.
    await waitFor(() => {
      expect(
        screen.getAllByText(
          en["person.intro.handoffOwner"].replace("{name}", "Sofia Meier"),
        ).length,
      ).toBe(2);
    });
    expect(screen.getByText(/^due /)).toBeTruthy();
    expect(
      screen.getByText(
        `${en["person.intro.stepCurrent"]} · ${en["person.intro.stepAwaitingAnswer"]}`,
      ),
    ).toBeTruthy();
  });

  it("hands the move back to the requester once the colleague has said yes", async () => {
    renderTab(
      graph,
      "en",
      {},
      {
        asks: [
          { ...ask, status: "accepted", decided_at: "2026-09-04T09:00:00Z" },
        ],
      },
    );

    await waitFor(() => {
      expect(
        screen.getAllByText(
          en["person.intro.handoffOwner"].replace("{name}", "Demo Admin"),
        ).length,
      ).toBe(2);
    });
    // The answer window has closed; nothing is due.
    expect(screen.queryByText(/^due /)).toBeNull();
    expect(
      screen.getByText(
        `${en["person.intro.stepDone"]} · ${en["person.intro.stateAccepted"]}`,
      ),
    ).toBeTruthy();
  });
});

// What moved lately is a dated list, newest first, and its head is the same
// change the strip's "why now" slot was read from.
describe("what changed lately", () => {
  const graph: PersonGraph = {
    person_id: PERSON,
    nodes: [anchor],
    edges: [],
    routes: [],
    groups_omitted: [],
  };

  it("flags the newest change as the reason to act", async () => {
    renderTab(
      graph,
      "en",
      {},
      {
        view: {
          relationship_changes: [
            { kind: "replied_after_gap", at: "2026-08-20T09:00:00Z", days: 41 },
            {
              kind: "warmed",
              at: "2026-08-25T09:00:00Z",
              from_bucket: "weak",
              to_bucket: "strong",
            },
          ],
        },
      },
    );

    const head = en["person.change.repliedAfterGap"].replace("{days}", "41");
    // The strip's slot and the list's head: one change, two places.
    await waitFor(() => {
      expect(screen.getAllByText(head).length).toBe(2);
    });
    // The label over the slot and the badge on the head say the same words.
    expect(screen.getAllByText(en["person.intro.stripWhyNow"]).length).toBe(2);
    expect(
      screen.getByText(
        en["person.change.warmed"]
          .replace("{from}", en["person.band.weak"])
          .replace("{to}", en["person.band.strong"]),
      ),
    ).toBeTruthy();
  });

  it("says nothing moved rather than drawing an empty list", async () => {
    renderTab(graph, "en", {}, { view: { relationship_changes: [] } });

    expect(
      await screen.findByText(en["person.network.noMoments"]),
    ).toBeTruthy();
    expect(screen.getByText(en["person.intro.stripNoMoment"])).toBeTruthy();
  });

  it("does not claim nothing moved when the section was withheld", async () => {
    renderTab(
      graph,
      "en",
      {},
      {
        view: { sections_omitted: ["relationship_changes"] },
      },
    );

    expect(await screen.findByText(en["state.withheld"])).toBeTruthy();
    expect(screen.queryByText(en["person.network.noMoments"])).toBeNull();
    expect(screen.getByText(en["person.intro.stripNoMoment"])).toBeTruthy();
  });
});

// The picture is the only route to recording an observed acquaintance: the
// server flags `suggest_edge` on peer nodes alone, the offer rides the map's
// panel, and the panel can only describe a node the layout placed. A peer the
// map does not place therefore takes the product's ONE writer of `works_with`
// off every screen, with the endpoint, the flag and the grant all still right.
describe("a peer the graph observed but nothing has recorded", () => {
  const PEER = "018f3a1b-0000-7000-8000-000000000030";
  const peer: PersonGraph["nodes"][number] = {
    id: `person:${PEER}`,
    type: "contact",
    group: "peer",
    label: "Rui Peer",
    person_id: PEER,
    suggest_edge: true,
  };

  function withPeer(): PersonGraph {
    return {
      person_id: PERSON,
      nodes: [anchor, peer],
      edges: [
        {
          from: `person:${PERSON}`,
          to: peer.id,
          strength_bucket: "strong",
          interactions_90d: 6,
        },
      ],
      routes: [],
      groups_omitted: [],
    } as PersonGraph;
  }

  it("can be selected on the map and offers the write", async () => {
    const user = userEvent.setup();
    renderTab(withPeer(), "en", { relationship: ["create"] });

    await user.click(await screen.findByRole("button", { name: /Rui Peer/ }));

    expect(
      await screen.findByRole("button", {
        name: en["person.graph.recordWorksWith"].replace("{name}", "Rui Peer"),
      }),
    ).toBeTruthy();
  });

  // The grant on that control is only observable while the control is
  // reachable. A map that placed no peer made this branch dead code, and the
  // agreement between the flag the server sets and the grant it demands
  // stopped being visible from any screen.
  it("offers no write to a seat that may not create a relationship", async () => {
    const user = userEvent.setup();
    const { seatAnswered } = renderTab(withPeer(), "en", {});

    await user.click(await screen.findByRole("button", { name: /Rui Peer/ }));

    // The seat has spoken, and React has drawn its answer. Without both, this
    // is a query against a page that has not read the grants yet, which is
    // absent for the same reason a loading page is — and would keep passing if
    // the control stopped consulting them.
    await seatAnswered;
    await act(async () => {});

    expect(
      screen.queryByRole("button", {
        name: en["person.graph.recordWorksWith"].replace("{name}", "Rui Peer"),
      }),
    ).toBeNull();
  });
});
