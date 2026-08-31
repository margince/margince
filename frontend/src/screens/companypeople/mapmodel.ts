import type { components } from "../../api/schema";
import type {
  MapBand,
  MapEdge,
  MapEngagement,
  MapLane,
  MapNode,
  RelationshipMapModel,
} from "../../design-system/relationshipmap.layout";

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
  return {
    id: `p:${seat.person_id}`,
    kind: "person",
    label: seat.full_name,
    engagement,
    engagementLabel: engagement ? copy.engagement[engagement] : undefined,
  };
}

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
  const known = new Set<string>(ROLES);
  for (const role of ROLES) {
    const seats = committee.seats.filter((seat) => seat.role === role);
    const ids: string[] = [];
    for (const seat of seats) {
      nodes.push(personNode(seat, copy));
      ids.push(`p:${seat.person_id}`);
      edges.push(...routeEdges(seat, copy));
      if (deal) {
        edges.push({
          id: `m:${seat.person_id}`,
          from: `p:${seat.person_id}`,
          to: `d:${deal.deal_id}`,
          kind: "membership",
          words: copy.onDeal,
        });
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

  // A seat with a role this board has no lane for is still a person the
  // summary counted. It gets a lane of its own rather than vanishing.
  const other = committee.seats.filter((seat) => !known.has(seat.role));
  if (other.length > 0) {
    for (const seat of other) {
      nodes.push(personNode(seat, copy));
      edges.push(...routeEdges(seat, copy));
    }
    lanes.push({
      id: "other",
      column: "right",
      label: copy.otherRoles,
      nodeIds: other.map((seat) => `p:${seat.person_id}`),
    });
  }
}
