import type { components } from "../../api/schema";
import type {
  MapBand,
  MapEdge,
  MapEngagement,
  MapLane,
  MapNode,
  RelationshipMapModel,
} from "../../design-system/relationshipmap.layout";
import { routeFor } from "../../design-system/relationshipmap.layout";
import type { IntroTarget } from "./introrequest";

// The wire read, as the picture the map draws.
//
// One place, and a pure function: the map is data-only by design, so if this
// lived inside the screen the states worth looking at — a hole in the team, a
// contact nobody can reach, a route through a colleague who has left — could
// only be seen by seeding an account that has them.

type Coverage = components["schemas"]["OrganizationCoverage"];
type Seat = components["schemas"]["OrganizationCoverageSeat"];

/** The lanes a committee reads in, and the order they read. */
const ROLES = [
  "champion",
  "economic_buyer",
  "influencer",
  "blocker",
  "user",
] as const;

export type MapCopy = Readonly<{
  /** What a seat says when the reader may not see who can reach them. */
  routesWithheld: string;
  ourSide: string;
  account: string;
  roles: Record<(typeof ROLES)[number], string>;
  otherRoles: string;
  /** "{role} missing" — the words in a gap node. */
  missing: (role: string) => string;
  assign: string;
  engagement: Record<MapEngagement, string>;
  /** How an edge reads, given which way the conversation is owed. */
  awaitingReply: string;
  theyReplied: string;
  neverWritten: string;
  onDeal: string;
  /** The verb on a person somebody on our side can actually reach. */
  askIntro: string;
}>;

/**
 * mapModelFromCoverage builds the picture.
 *
 * Colleagues are derived from the seats' own routes rather than listed
 * separately: a colleague with no route to anybody here would be a node with no
 * edges, which tells a reader nothing and costs them a tab stop.
 */
export function mapModelFromCoverage(
  coverage: Coverage,
  accountName: string,
  copy: MapCopy,
): RelationshipMapModel {
  const committee = coverage.committee;
  if (!committee) {
    return { nodes: [], lanes: [], edges: [] };
  }
  const nodes: MapNode[] = [];
  const lanes: MapLane[] = [];
  const edges: MapEdge[] = [];

  // Our side, in the order the routes rank them: strongest relationship first,
  // which is the order a reader deciding whom to ask wants to read.
  const colleagues = new Map<string, string>();
  for (const seat of committee.seats) {
    for (const route of seat.routes?.top ?? []) {
      if (!colleagues.has(route.user_id)) {
        colleagues.set(route.user_id, route.display_name);
      }
    }
  }
  for (const [id, name] of colleagues) {
    nodes.push({ id: `u:${id}`, kind: "user", label: name });
  }
  if (colleagues.size > 0) {
    lanes.push({
      id: "ourside",
      column: "left",
      label: copy.ourSide,
      nodeIds: [...colleagues.keys()].map((id) => `u:${id}`),
    });
  }

  // The account, and the deal the committee belongs to.
  nodes.push({
    id: "org",
    kind: "organization",
    label: accountName,
    sublabel: copy.account,
  });
  const centre = ["org"];
  const deal = coverage.deals.find(
    (candidate) => candidate.deal_id === coverage.selected_deal_id,
  );
  if (deal) {
    nodes.push({ id: `d:${deal.deal_id}`, kind: "deal", label: deal.name });
    centre.push(`d:${deal.deal_id}`);
  }
  lanes.push({ id: "centre", column: "center", label: "", nodeIds: centre });

  // Their people, by role, with a gap where a critical role is unheld.
  buildRoleLanes(committee, deal, copy, nodes, lanes, edges);

  return { nodes, lanes, edges };
}

function personNode(seat: Seat, copy: MapCopy): MapNode {
  const engagement = seat.engagement as MapEngagement | undefined;
  // The verb goes ONLY on a seat somebody can actually reach. Asking for an
  // introduction from nobody is not a move, and offering it on a contact with
  // no route would send the reader to a dialog that can only refuse — the
  // endpoint requires a recorded route and answers 404 without one.
  const reachable = (seat.routes?.top ?? []).length > 0;
  return {
    id: `p:${seat.person_id}`,
    kind: "person",
    label: seat.full_name,
    // ABSENT routes and EMPTY routes are opposite facts. Absent means the
    // reader may not ask who can reach this person; empty is the answer that
    // nobody can. Drawing both as a person with no line would report a
    // withholding as a recorded absence.
    sublabel: seat.routes ? undefined : copy.routesWithheld,
    engagement,
    engagementLabel: engagement ? copy.engagement[engagement] : undefined,
    actions: reachable ? [{ id: ASK_INTRO, label: copy.askIntro }] : undefined,
  };
}

/** The action a person node offers when somebody on our side can reach them. */
export const ASK_INTRO = "ask_intro";

/**
 * routeEdges turns one seat's colleagues into lines.
 *
 * The words carry the direction, because a line cannot: "awaiting reply" and
 * "they replied" are opposite next moves and would otherwise be one grey line
 * apiece. The band the server already decided is used as it stands — a second
 * banding here would let the map disagree with the row beneath it.
 */
function routeEdges(seat: Seat, copy: MapCopy): MapEdge[] {
  return (seat.routes?.top ?? []).map((route) => ({
    id: `e:${route.user_id}:${seat.person_id}`,
    from: `u:${route.user_id}`,
    to: `p:${seat.person_id}`,
    kind: "route" as const,
    band: route.strength_bucket as MapBand,
    lastAt: route.last_interaction_at ?? null,
    words: wordsFor(seat.engagement as MapEngagement | undefined, copy),
  }));
}

function wordsFor(
  engagement: MapEngagement | undefined,
  copy: MapCopy,
): string {
  if (engagement === "answered") {
    return copy.theyReplied;
  }
  if (engagement === "no_reply") {
    return copy.awaitingReply;
  }
  return copy.neverWritten;
}

/**
 * buildRoleLanes walks the committee once, laying out one lane per role.
 *
 * A lane appears only when it holds somebody or a gap: an empty heading reads
 * as a section that failed to load rather than as a role nobody holds.
 */
function buildRoleLanes(
  committee: NonNullable<Coverage["committee"]>,
  deal: Coverage["deals"][number] | undefined,
  copy: MapCopy,
  nodes: MapNode[],
  lanes: MapLane[],
  edges: MapEdge[],
): void {
  // One node per PERSON, in the first role they hold. A stakeholder can sit on
  // a deal twice (the table's key is deal, person AND role), and drawing them
  // once per role produced two nodes with the same id, two identical edges and
  // two React keys — a picture that cannot say which of the two a click meant.
  const drawn = new Set<string>();
  for (const role of ROLES) {
    const seats = committee.seats.filter(
      (seat) => seat.role === role && !drawn.has(seat.person_id),
    );
    for (const seat of seats) {
      drawn.add(seat.person_id);
    }
    const ids: string[] = [];
    for (const seat of seats) {
      nodes.push(personNode(seat, copy));
      ids.push(`p:${seat.person_id}`);
      edges.push(...routeEdges(seat, copy));
      if (deal) {
        edges.push(dealEdge(seat.person_id, deal.deal_id, copy));
      }
    }
    if (seats.length === 0 && committee.gaps.includes(role)) {
      const id = `gap:${role}`;
      nodes.push({
        id,
        kind: "gap",
        label: copy.missing(copy.roles[role]),
        sublabel: copy.assign,
      });
      ids.push(id);
    }
    if (ids.length > 0) {
      lanes.push({
        id: role,
        column: "right",
        label: copy.roles[role],
        nodeIds: ids,
      });
    }
  }

  buildOtherLane(committee, deal, copy, drawn, nodes, lanes, edges);
}

/**
 * buildOtherLane catches the seats no named lane claimed.
 *
 * `role` is a free string on the wire, and a seat carrying one this board has
 * no lane for is still a person the summary counted — dropping it sends a
 * reader looking for somebody the picture never drew.
 */
function buildOtherLane(
  committee: NonNullable<Coverage["committee"]>,
  deal: Coverage["deals"][number] | undefined,
  copy: MapCopy,
  drawn: Set<string>,
  nodes: MapNode[],
  lanes: MapLane[],
  edges: MapEdge[],
): void {
  const known = new Set<string>(ROLES);
  const other = committee.seats.filter(
    (seat) => !known.has(seat.role) && !drawn.has(seat.person_id),
  );
  if (other.length === 0) {
    return;
  }
  for (const seat of other) {
    drawn.add(seat.person_id);
    nodes.push(personNode(seat, copy));
    edges.push(...routeEdges(seat, copy));
    if (deal) {
      edges.push(dealEdge(seat.person_id, deal.deal_id, copy));
    }
  }
  lanes.push({
    id: "other",
    column: "right",
    label: copy.otherRoles,
    nodeIds: other.map((seat) => `p:${seat.person_id}`),
  });
}

/** dealEdge joins one seat to the deal it sits on. */
function dealEdge(personId: string, dealId: string, copy: MapCopy): MapEdge {
  return {
    id: `m:${personId}`,
    from: `p:${personId}`,
    to: `d:${dealId}`,
    kind: "membership",
    words: copy.onDeal,
  };
}

/**
 * introTargetFor names the colleague to ask, from the map the reader is
 * looking at.
 *
 * The STRONGEST route, which is the one the panel has already named as the
 * best way in — so the dialog asks about the person the reader just read
 * about, rather than whichever edge happened to be first.
 *
 * It refuses anything that is not a PERSON. A route edge runs colleague →
 * person, so reading `from` as the colleague is only true for a person focus;
 * on a colleague focus the same edge points the other way and this would ask
 * the contact to introduce the reader to their own colleague. Today the action
 * only sits on person nodes, which makes that unreachable — and a function
 * that is correct only because of where it happens to be called is one the
 * next caller breaks silently.
 */
export function introTargetFor(
  model: RelationshipMapModel,
  nodeId: string,
): IntroTarget | null {
  const person = model.nodes.find((node) => node.id === nodeId);
  if (person?.kind !== "person") {
    return null;
  }
  const { route } = routeFor(model, nodeId);
  const best = route
    ? model.edges.find((edge) => edge.id === route.edgeIds[0])
    : null;
  const colleague = best && model.nodes.find((node) => node.id === best.from);
  if (!best || colleague?.kind !== "user") {
    return null;
  }
  return {
    // The ids the map draws with are PREFIXED so a person and a colleague
    // cannot collide; the endpoint wants the bare uuid. Stripped by the node's
    // own kind rather than by pattern, so a value that merely starts with the
    // letters cannot be trimmed into a different record.
    personId: nodeId.slice("p:".length),
    personName: person.label,
    viaUserId: colleague.id.slice("u:".length),
    viaName: colleague.label,
  };
}
