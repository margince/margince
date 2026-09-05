// The map's geometry and its route arithmetic, as pure functions.
//
// Separate from the drawing so both can be tested for what they actually
// promise: the same model must place the same nodes at the same coordinates on
// every read, and the highlighted route must be the strongest one the edges
// support rather than whichever the renderer happened to walk first.

/** What a node stands for, which decides its shape rather than its colour. */
export type MapNodeKind = "user" | "person" | "organization" | "deal" | "gap";

/**
 * The three bands a ROUTE is shown in.
 *
 * Not the four the relationship score supports: a reader deciding whom to ask
 * needs a coarser answer than the score carries, and the two vocabularies are
 * deliberately different lists.
 */
export type MapBand = "strong" | "developing" | "cold";

export type MapEngagement = "waiting" | "answered" | "no_reply" | "untried";

export type MapNode = Readonly<{
  id: string;
  kind: MapNodeKind;
  label: string;
  sublabel?: string;
  engagement?: MapEngagement;
  /** The engagement in the reader's own language; the map never translates. */
  engagementLabel?: string;
  addedBySearch?: boolean;
  actions?: readonly MapAction[];
}>;

export type MapAction = Readonly<{
  id: string;
  label: string;
  primary?: boolean;
}>;

export type MapLane = Readonly<{
  id: string;
  column: "left" | "center" | "right";
  label: string;
  /** The drawn order. The layout never sorts — the caller's order is the order. */
  nodeIds: readonly string[];
}>;

export type MapEdge = Readonly<{
  id: string;
  from: string;
  to: string;
  kind: "route" | "membership";
  band?: MapBand;
  lastAt?: string | null;
  /** What this edge means, in the reader's words. Never inferred from colour. */
  words: string;
}>;

export type RelationshipMapModel = Readonly<{
  nodes: readonly MapNode[];
  lanes: readonly MapLane[];
  edges: readonly MapEdge[];
}>;

// The drawing's constants, in the SVG's own coordinate space. Exported because
// the tests assert against them rather than against numbers typed twice.
export const PAD = 16;
export const COL_W = { left: 184, center: 200, right: 184 } as const;
export const GUTTER = 72;
export const NODE_H = {
  user: 40,
  person: 60,
  organization: 48,
  deal: 44,
  gap: 60,
  more: 28,
} as const;
export const ROW_GAP = 8;
export const LANE_GAP = 20;
export const LANE_HEAD_H = 20;
export const CENTER_GAP = 24;
/** How many nodes a right-hand lane draws before it offers the rest. */
export const LANE_CAP = 8;
export const NAME_CHARS = 22;
export const WIDTH =
  PAD * 2 + COL_W.left + GUTTER + COL_W.center + GUTTER + COL_W.right;

const COLUMN_X = {
  left: PAD,
  center: PAD + COL_W.left + GUTTER,
  right: PAD + COL_W.left + GUTTER + COL_W.center + GUTTER,
} as const;

export type Placed = Readonly<{
  id: string;
  x: number;
  y: number;
  w: number;
  h: number;
  kind: MapNodeKind | "more";
  laneId: string;
}>;

export type LaneHead = Readonly<{
  id: string;
  x: number;
  y: number;
  label: string;
  hidden: number;
}>;

export type Layout = Readonly<{
  width: number;
  height: number;
  placed: readonly Placed[];
  heads: readonly LaneHead[];
}>;

/**
 * layout places every node the lanes name.
 *
 * Deterministic by construction: a lane's own order is the drawn order and the
 * height is a function of the counts, so the same model draws the same picture
 * on every read. A force simulation would move every node whenever one arrived,
 * and a reader who has learnt where to look would have to learn again.
 */
export function layout(
  model: RelationshipMapModel,
  expanded: ReadonlySet<string> = new Set(),
): Layout {
  const byId = new Map(model.nodes.map((node) => [node.id, node]));
  const placed: Placed[] = [];
  const heads: LaneHead[] = [];

  const columns = { left: 0, center: 0, right: 0 };
  const laneOf = (column: "left" | "center" | "right") =>
    model.lanes.filter((lane) => lane.column === column);

  for (const column of ["left", "right"] as const) {
    columns[column] = stackColumn(
      column,
      laneOf(column),
      byId,
      expanded,
      placed,
      heads,
    );
  }

  // The centre is placed last and centred against the taller side, so the
  // account and its deal sit level with the people they join rather than at the
  // top of a column that happens to be short.
  const centreNodes = laneOf("center").flatMap((lane) =>
    lane.nodeIds.map((id) => ({ id, laneId: lane.id })),
  );
  const centreHeight = centreNodes.reduce(
    (total, node, index) =>
      total +
      NODE_H[byId.get(node.id)?.kind ?? "organization"] +
      (index > 0 ? CENTER_GAP : 0),
    0,
  );
  const tallest = Math.max(columns.left, columns.right, centreHeight + PAD * 2);
  let centreY = Math.max(PAD, (tallest - centreHeight) / 2);
  for (const node of centreNodes) {
    const kind = byId.get(node.id)?.kind ?? "organization";
    const h = NODE_H[kind];
    placed.push({
      id: node.id,
      x: COLUMN_X.center,
      y: centreY,
      w: COL_W.center,
      h,
      kind,
      laneId: node.laneId,
    });
    centreY += h + CENTER_GAP;
  }

  return { width: WIDTH, height: Math.ceil(tallest), placed, heads };
}

/**
 * stackColumn lays one side column out top to bottom and reports where it ends.
 *
 * Its own function because the two side columns are the same walk and the
 * centre is a different one: folding all three into the caller made a reader
 * hold three shapes at once to check any of them.
 */
function stackColumn(
  column: "left" | "right",
  lanes: readonly MapLane[],
  byId: ReadonlyMap<string, MapNode>,
  expanded: ReadonlySet<string>,
  placed: Placed[],
  heads: LaneHead[],
): number {
  let y = PAD;
  for (const lane of lanes) {
    // A lane with nobody in it draws nothing. A heading over empty space reads
    // as a section that failed to load rather than as a role nobody holds —
    // the caller says THAT with a gap node.
    if (lane.nodeIds.length === 0) {
      continue;
    }
    const shown = expanded.has(lane.id)
      ? lane.nodeIds
      : lane.nodeIds.slice(0, LANE_CAP);
    const hidden = lane.nodeIds.length - shown.length;
    heads.push({
      id: lane.id,
      x: COLUMN_X[column],
      y,
      label: lane.label,
      hidden,
    });
    y += LANE_HEAD_H;
    for (const id of shown) {
      const kind = byId.get(id)?.kind ?? "person";
      const h = NODE_H[kind];
      placed.push({
        id,
        x: COLUMN_X[column],
        y,
        w: COL_W[column],
        h,
        kind,
        laneId: lane.id,
      });
      y += h + ROW_GAP;
    }
    if (hidden > 0) {
      placed.push({
        id: `more:${lane.id}`,
        x: COLUMN_X[column],
        y,
        w: COL_W[column],
        h: NODE_H.more,
        kind: "more",
        laneId: lane.id,
      });
      y += NODE_H.more + ROW_GAP;
    }
    y += LANE_GAP;
  }
  return y;
}

const BAND_ORDER: Record<MapBand, number> = {
  strong: 3,
  developing: 2,
  cold: 1,
};

export type Route = Readonly<{
  /** Every node the focus touches, including itself. */
  related: ReadonlySet<string>;
  /** The one path worth drawing, or null when the focus has no route. */
  route: Readonly<{
    nodeIds: readonly string[];
    edgeIds: readonly string[];
  }> | null;
}>;

/**
 * routeFor decides what a focused node lights up.
 *
 * The BEST route, not every route: a reader asking "how do I reach this
 * person" is choosing one colleague to ask, and drawing four equally is
 * leaving the choice to them again. Strongest band wins; a tie goes to the
 * most recent exchange, and a tie there to the edge id so the same model
 * always lights the same path.
 */
export function routeFor(
  model: RelationshipMapModel,
  focusId: string | null,
): Route {
  const none: Route = { related: new Set(), route: null };
  if (!focusId) {
    return none;
  }
  const node = model.nodes.find((candidate) => candidate.id === focusId);
  if (!node) {
    // A stale focus (a URL naming a contact this account no longer has) must
    // not blank the map: nothing is related, so nothing fades.
    return none;
  }
  const routes = model.edges.filter((edge) => edge.kind === "route");
  const memberships = model.edges.filter((edge) => edge.kind === "membership");

  if (node.kind === "person") {
    return personRoute(focusId, routes, memberships);
  }
  if (node.kind === "user") {
    return colleagueRoute(focusId, routes, memberships);
  }
  if (node.kind === "deal") {
    const related = new Set<string>([focusId]);
    for (const edge of memberships) {
      if (edge.to === focusId) {
        related.add(edge.from);
      }
    }
    return { related, route: null };
  }
  // An organization or a gap relates only to itself: the account is what
  // everything is already about, and a gap is a hole rather than a node with
  // edges.
  return { related: new Set([focusId]), route: null };
}

/** The walk into one of their people: who can reach them, and by which route. */
function personRoute(
  focusId: string,
  routes: readonly MapEdge[],
  memberships: readonly MapEdge[],
): Route {
  {
    const into = routes.filter((edge) => edge.to === focusId);
    const best = strongest(into);
    const related = new Set<string>([focusId]);
    for (const edge of into) {
      related.add(edge.from);
    }
    const onward = memberships.filter((edge) => edge.from === focusId);
    for (const edge of onward) {
      related.add(edge.to);
    }
    if (!best) {
      return { related, route: null };
    }
    return {
      related,
      route: {
        nodeIds: [best.from, focusId, ...onward.map((edge) => edge.to)],
        edgeIds: [best.id, ...onward.map((edge) => edge.id)],
      },
    };
  }
}

/** The same walk from our side: whom this colleague can reach. */
function colleagueRoute(
  focusId: string,
  routes: readonly MapEdge[],
  memberships: readonly MapEdge[],
): Route {
  {
    const out = routes.filter((edge) => edge.from === focusId);
    const best = strongest(out);
    const related = new Set<string>([focusId]);
    for (const edge of out) {
      related.add(edge.to);
    }
    if (!best) {
      return { related, route: null };
    }
    const onward = memberships.filter((edge) => edge.from === best.to);
    for (const edge of onward) {
      related.add(edge.to);
    }
    return {
      related,
      route: {
        nodeIds: [focusId, best.to, ...onward.map((edge) => edge.to)],
        edgeIds: [best.id, ...onward.map((edge) => edge.id)],
      },
    };
  }
}

/**
 * strongest picks the edge a reader should act on, by `beats` below.
 *
 * Exported because a caller that narrows the candidates first — the account
 * map's introduction, which may not ask the reader for a favour from
 * themselves — still has to rank what is left the way the drawing does. A
 * second comparison there would light one route on the picture and open a
 * dialog about another.
 */
export function strongest(edges: readonly MapEdge[]): MapEdge | null {
  let best: MapEdge | null = null;
  for (const edge of edges) {
    if (!best || beats(edge, best)) {
      best = edge;
    }
  }
  return best;
}

/**
 * beats is the one comparison, spelled once: strongest band, then the most
 * recent exchange, then the edge id so the same model always lights the same
 * path rather than whichever the scan reached first.
 */
function beats(edge: MapEdge, best: MapEdge): boolean {
  const band =
    BAND_ORDER[edge.band ?? "cold"] - BAND_ORDER[best.band ?? "cold"];
  if (band !== 0) {
    return band > 0;
  }
  const a = edge.lastAt ? Date.parse(edge.lastAt) : Number.NEGATIVE_INFINITY;
  const b = best.lastAt ? Date.parse(best.lastAt) : Number.NEGATIVE_INFINITY;
  if (a !== b) {
    return a > b;
  }
  return edge.id < best.id;
}

/**
 * travelOrder is the order the keyboard walks: lane by lane, top to bottom, in
 * the order the lanes were declared.
 */
export function travelOrder(layoutResult: Layout): readonly string[] {
  return layoutResult.placed.map((node) => node.id);
}

/** truncate cuts a label to fit its node, deterministically. */
export function truncate(text: string, chars = NAME_CHARS): string {
  return text.length <= chars ? text : `${text.slice(0, chars - 1)}…`;
}
