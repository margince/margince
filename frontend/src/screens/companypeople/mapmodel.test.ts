// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, test } from "vitest";
import type { components } from "../../api/schema";
import { introTargetFor, type MapCopy, mapModelFromCoverage } from "./mapmodel";

type Coverage = components["schemas"]["OrganizationCoverage"];

const COPY: MapCopy = {
  routesWithheld: "Hidden from you",
  ourSide: "Our side",
  account: "Account",
  roles: {
    champion: "Champion",
    economic_buyer: "Economic buyer",
    influencer: "Influencers",
    blocker: "Blockers",
    user: "Users",
  },
  otherRoles: "Other roles",
  missing: (role) => `${role} missing`,
  assign: "Assign",
  engagement: {
    waiting: "Needs reply",
    answered: "Answered",
    no_reply: "No reply",
    untried: "Not approached",
  },
  awaitingReply: "awaiting reply",
  replyOwed: "reply owed",
  theyReplied: "they replied",
  neverWritten: "never written to",
  onDeal: "on the deal",
  askIntro: "Ask for an intro",
};

function coverage(over: Partial<Coverage> = {}): Coverage {
  return {
    as_of: "2026-08-31T09:00:00Z",
    summary: {
      contacts_total: 3,
      waiting: 0,
      answered: 1,
      no_reply: 0,
      untried: 2,
    },
    deals: [{ deal_id: "d-1", name: "Retrofit 2026" }],
    selected_deal_id: "d-1",
    completeness: { committee_read: true },
    committee: {
      seats: [
        {
          person_id: "p-1",
          full_name: "Philipp Königs",
          role: "economic_buyer",
          engagement: "untried",
          routes: {
            top: [
              {
                user_id: "u-1",
                display_name: "Sofia Meier",
                strength_bucket: "developing",
                last_interaction_at: "2026-08-20T09:00:00Z",
              },
            ],
            remainder: 0,
            untried: false,
          },
        },
      ],
      gaps: ["champion"],
      unlisted_seats: 0,
    },
    ...over,
  } as Coverage;
}

test("puts colleagues left, the account and deal centre, their people right", () => {
  const model = mapModelFromCoverage(coverage(), "Brandt GmbH", COPY);
  const column = (id: string) =>
    model.lanes.find((lane) => lane.id === id)?.column;
  expect(column("ourside")).toBe("left");
  expect(column("centre")).toBe("center");
  expect(column("economic_buyer")).toBe("right");
});

// A colleague reaches the picture through a ROUTE. One with no route to
// anybody here would be a node with no edges — nothing for a reader to learn,
// and one more stop on the keyboard walk.
test("takes our side from the routes rather than listing everyone", () => {
  const model = mapModelFromCoverage(coverage(), "Brandt GmbH", COPY);
  const ourSide = model.lanes.find((lane) => lane.id === "ourside");
  expect(ourSide?.nodeIds).toEqual(["u:u-1"]);
  expect(model.nodes.find((n) => n.id === "u:u-1")?.label).toBe("Sofia Meier");
});

test("draws a gap where a critical role is unheld", () => {
  const model = mapModelFromCoverage(coverage(), "Brandt GmbH", COPY);
  const gap = model.nodes.find((node) => node.id === "gap:champion");
  expect(gap?.kind).toBe("gap");
  expect(gap?.label).toBe("Champion missing");
  // In the lane that would hold them, not somewhere else.
  expect(model.lanes.find((lane) => lane.id === "champion")?.nodeIds).toContain(
    "gap:champion",
  );
});

// The server empties `gaps` when it cannot judge the committee. The map must
// not invent a hole from the seats it happens to hold.
test("draws no gap when the server named none", () => {
  const model = mapModelFromCoverage(
    coverage({
      committee: { seats: [], gaps: [], unlisted_seats: 3 },
    }),
    "Brandt GmbH",
    COPY,
  );
  expect(model.nodes.filter((node) => node.kind === "gap")).toHaveLength(0);
});

test("carries the server's own band rather than banding again", () => {
  const model = mapModelFromCoverage(coverage(), "Brandt GmbH", COPY);
  const edge = model.edges.find((candidate) => candidate.kind === "route");
  expect(edge?.band).toBe("developing");
  expect(edge?.from).toBe("u:u-1");
  expect(edge?.to).toBe("p:p-1");
});

// The words carry the direction, because a line cannot: "awaiting reply" and
// "they replied" are opposite next moves and would otherwise be one grey line
// apiece.
test("says which way the conversation is owed", () => {
  const answered = mapModelFromCoverage(
    coverage({
      committee: {
        seats: [
          {
            ...coverage().committee?.seats[0],
            engagement: "answered",
          },
        ],
        gaps: [],
        unlisted_seats: 0,
      },
    } as Partial<Coverage>),
    "Brandt GmbH",
    COPY,
  );
  expect(answered.edges.find((edge) => edge.kind === "route")?.words).toBe(
    "they replied",
  );
});

test("joins each seat to the deal it sits on", () => {
  const model = mapModelFromCoverage(coverage(), "Brandt GmbH", COPY);
  const membership = model.edges.find((edge) => edge.kind === "membership");
  expect(membership?.from).toBe("p:p-1");
  expect(membership?.to).toBe("d:d-1");
});

// A seat with a role the board has no lane for is still a person the summary
// counted. Dropping it sends a reader looking for somebody never drawn.
test("gives an unrecognised role a lane of its own", () => {
  const model = mapModelFromCoverage(
    coverage({
      committee: {
        seats: [
          {
            person_id: "p-9",
            full_name: "Sam Consultant",
            role: "technical_advisor",
            engagement: "answered",
          },
        ],
        gaps: [],
        unlisted_seats: 0,
      },
    } as Partial<Coverage>),
    "Brandt GmbH",
    COPY,
  );
  expect(model.lanes.find((lane) => lane.id === "other")?.nodeIds).toEqual([
    "p:p-9",
  ]);
});

// A caller without the activity grant gets no routes at all. The map then has
// no lines to draw, and must not pretend the account has no colleagues.
test("draws nobody on our side when routes were withheld", () => {
  const model = mapModelFromCoverage(
    coverage({
      committee: {
        seats: [
          {
            person_id: "p-1",
            full_name: "Philipp Königs",
            role: "economic_buyer",
            engagement: "untried",
          },
        ],
        gaps: [],
        unlisted_seats: 0,
      },
    } as Partial<Coverage>),
    "Brandt GmbH",
    COPY,
  );
  expect(model.lanes.find((lane) => lane.id === "ourside")).toBeUndefined();
  expect(model.edges.filter((edge) => edge.kind === "route")).toHaveLength(0);
  // The seat is still drawn: being unreachable is the finding.
  expect(model.nodes.find((node) => node.id === "p:p-1")).toBeDefined();
});

// A committee the reader may not read draws nothing rather than an empty
// picture that reads as "this account has nobody".
test("draws nothing at all when the committee was withheld", () => {
  const model = mapModelFromCoverage(
    coverage({ committee: undefined, completeness: { committee_read: false } }),
    "Brandt GmbH",
    COPY,
  );
  expect(model.nodes).toHaveLength(0);
  expect(model.lanes).toHaveLength(0);
});

// Absent routes and empty routes are opposite facts: absent means the reader
// may not ask who can reach this person, empty is the answer that nobody can.
// Drawing both as a person with no line reports a withholding as an absence.
test("says routes were withheld rather than showing nobody", () => {
  const withheld = mapModelFromCoverage(
    coverage({
      committee: {
        seats: [
          {
            person_id: "p-1",
            full_name: "Philipp Königs",
            role: "economic_buyer",
            engagement: "untried",
          },
        ],
        gaps: [],
        unlisted_seats: 0,
      },
    } as Partial<Coverage>),
    "Brandt GmbH",
    COPY,
  );
  expect(withheld.nodes.find((n) => n.id === "p:p-1")?.sublabel).toBe(
    "Hidden from you",
  );

  const answered = mapModelFromCoverage(coverage(), "Brandt GmbH", COPY);
  // A seat whose routes WERE readable says nothing extra.
  expect(
    answered.nodes.find((n) => n.id === "p:p-1")?.sublabel,
  ).toBeUndefined();
});

// A stakeholder can sit on one deal twice — the table's key is deal, person
// AND role. Drawing them once per role produced two nodes with the same id,
// two identical edges and two React keys.
test("draws a person once even when they hold two roles", () => {
  const twice = mapModelFromCoverage(
    coverage({
      committee: {
        seats: [
          {
            person_id: "p-1",
            full_name: "Philipp Königs",
            role: "champion",
            engagement: "answered",
          },
          {
            person_id: "p-1",
            full_name: "Philipp Königs",
            role: "economic_buyer",
            engagement: "answered",
          },
        ],
        gaps: [],
        unlisted_seats: 0,
      },
    } as Partial<Coverage>),
    "Brandt GmbH",
    COPY,
  );
  expect(twice.nodes.filter((node) => node.id === "p:p-1")).toHaveLength(1);
  expect(new Set(twice.edges.map((edge) => edge.id)).size).toBe(
    twice.edges.length,
  );
});

// A stakeholder whose role has no lane of its own is still ON the deal.
// Leaving the line off drew them floating beside it.
test("joins an unrecognised role to the deal like every other seat", () => {
  const model = mapModelFromCoverage(
    coverage({
      committee: {
        seats: [
          {
            person_id: "p-9",
            full_name: "Sam Consultant",
            role: "technical_advisor",
            engagement: "answered",
          },
        ],
        gaps: [],
        unlisted_seats: 0,
      },
    } as Partial<Coverage>),
    "Brandt GmbH",
    COPY,
  );
  expect(
    model.edges.find(
      (edge) => edge.kind === "membership" && edge.from === "p:p-9",
    ),
  ).toBeDefined();
});

// The verb goes ONLY on somebody who can actually be reached. Offering it on a
// contact with no route sends the reader to a dialog that can only refuse: the
// endpoint requires a recorded route and answers 404 without one.
test("offers the intro verb only where a route exists", () => {
  const reachable = mapModelFromCoverage(coverage(), "Brandt GmbH", COPY);
  expect(
    reachable.nodes.find((node) => node.id === "p:p-1")?.actions?.[0]?.id,
  ).toBe("ask_intro");

  const unreachable = mapModelFromCoverage(
    coverage({
      committee: {
        seats: [
          {
            person_id: "p-1",
            full_name: "Philipp Königs",
            role: "economic_buyer",
            engagement: "untried",
            routes: { top: [], remainder: 0, untried: true },
          },
        ],
        gaps: [],
        unlisted_seats: 0,
      },
    } as Partial<Coverage>),
    "Brandt GmbH",
    COPY,
  );
  expect(
    unreachable.nodes.find((node) => node.id === "p:p-1")?.actions,
  ).toBeUndefined();
});

// The dialog asks the COLLEAGUE to introduce the reader to the CONTACT, and a
// route edge runs colleague → person. Reading the ends the other way round
// would ask the customer's own CFO to introduce us to our colleague.
test("names the colleague to ask and the contact to be met, in that order", () => {
  const model = mapModelFromCoverage(coverage(), "Brandt GmbH", COPY);
  expect(introTargetFor(model, "p:p-1")).toEqual({
    personId: "p-1",
    personName: "Philipp Königs",
    viaUserId: "u-1",
    viaName: "Sofia Meier",
  });
});

// A route edge points one way, so which end is the colleague depends on which
// end was selected. Asked about a COLLEAGUE, this must refuse rather than read
// the edge backwards — today nothing calls it that way, and a function correct
// only because of where it happens to be called is one the next caller breaks.
test("refuses a node that is not a person", () => {
  const model = mapModelFromCoverage(coverage(), "Brandt GmbH", COPY);
  for (const id of ["u:u-1", "org", "d:d-1", "gap:champion", "p:missing"]) {
    expect(introTargetFor(model, id)).toBeNull();
  }
});
