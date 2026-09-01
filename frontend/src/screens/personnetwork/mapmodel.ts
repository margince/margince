// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The person graph, drawn as the shared relationship map.
//
// One picture language across the product: the account page already draws its
// committee this way, and a second diagram for the same question — who reaches
// this person, and how warmly — would be two answers a reader has to reconcile.
//
// Pure, so the drawing is testable without a DOM. Every word arrives from the
// caller: the map never translates.

import type { components } from "../../api/schema";
import type {
  MapBand,
  MapEdge,
  MapLane,
  MapNode,
  RelationshipMapModel,
} from "../../design-system/relationshipmap.layout";

type Graph = components["schemas"]["PersonGraph"];
type GraphNode = components["schemas"]["PersonGraphNode"];

/** The words the model needs, translated by the caller. */
export type MapCopy = Readonly<{
  ourTeam: string;
  theirCompany: string;
  target: string;
  useThisRoute: string;
  withheldDirect: string;
  withheldAccount: string;
  edgeDirect: (name: string) => string;
  edgeAccount: (name: string) => string;
}>;

/**
 * bandOf collapses the score's four buckets into the three a route is drawn in.
 *
 * `none` is OMITTED rather than drawn as `cold`: an edge the score could not
 * measure and an edge it measured as weak are different claims, and drawing
 * them alike would put a relationship on screen that nothing observed.
 */
export function bandOf(bucket: string | undefined): MapBand | undefined {
  switch (bucket) {
    case "strong":
      return "strong";
    case "moderate":
      return "developing";
    case "weak":
      return "cold";
    default:
      return undefined;
  }
}

/**
 * mapModelFromPersonGraph turns one graph payload into the map's model.
 *
 * Three lanes, matching the question a reader is asking: our colleagues on the
 * left, the contact's own company in the middle, and the contact themselves on
 * the right.
 */
export function mapModelFromPersonGraph(
  graph: Graph,
  copy: MapCopy,
): RelationshipMapModel {
  const nodes = graph.nodes ?? [];
  const anchor = nodes.find((n) => n.group === "anchor");
  const colleagues = nodes.filter((n) => n.group === "direct");
  const account = nodes.filter((n) => n.group === "account");

  // Only a colleague can carry an introduction, so only a colleague's node
  // offers the action. A contact at the target's company is context a reader
  // routes THROUGH, never somebody this product asks.
  const mapNodes: MapNode[] = [
    ...colleagues.map((n) => colleagueNode(n, copy)),
    ...account.map((n) => contactNode(n, "person")),
    ...(anchor ? [contactNode(anchor, "person")] : []),
  ];

  // Left: our colleagues. Centre: the contact themselves, because THEY are the
  // hub this whole picture is about. Right: the other people at their company,
  // who are the routes THROUGH.
  //
  // The centre is one node by design — the component stacks it vertically with
  // a wide gap, so a list there grows the drawing without adding a lane head.
  // Putting the twelve account contacts in it drew a 1000px column with no
  // heading and two of the three lanes missing.
  const lanes: MapLane[] = [
    {
      id: "ours",
      column: "left",
      label: copy.ourTeam,
      nodeIds: colleagues.map((n) => n.id),
    },
    {
      id: "target",
      column: "center",
      label: copy.target,
      nodeIds: anchor ? [anchor.id] : [],
    },
    {
      id: "theirs",
      column: "right",
      label: copy.theirCompany,
      nodeIds: account.map((n) => n.id),
    },
  ];

  const drawn = new Set(mapNodes.map((n) => n.id));
  const edges: MapEdge[] = [];
  for (const edge of graph.edges ?? []) {
    // An edge to a node the graph withheld has nothing to join. Drawing it
    // would put a line on screen reaching a name the reader may not see.
    if (!drawn.has(edge.from) || !drawn.has(edge.to)) {
      continue;
    }
    const band = bandOf(edge.strength_bucket);
    if (!band) {
      continue;
    }
    const reachesTarget = anchor !== undefined && edge.to === anchor.id;
    edges.push({
      id: `${edge.from}->${edge.to}`,
      from: edge.from,
      to: edge.to,
      kind: "route",
      band,
      lastAt: edge.last_at ?? null,
      words: reachesTarget
        ? copy.edgeDirect(labelOf(nodes, edge.from))
        : copy.edgeAccount(labelOf(nodes, edge.to)),
    });
  }

  return { nodes: mapNodes, lanes, edges };
}

/**
 * completenessText states what the picture is missing, in one sentence.
 *
 * The graph already reports withheld groups and a dropped count; a map that
 * dropped those would show a partial picture as a complete one, which is the
 * one thing this surface must never do.
 */
export function completenessText(
  graph: Graph,
  copy: MapCopy,
  droppedText: (n: number) => string,
): string {
  const parts: string[] = [];
  const omitted = graph.groups_omitted ?? [];
  if (omitted.includes("direct")) {
    parts.push(copy.withheldDirect);
  }
  if (omitted.includes("account")) {
    parts.push(copy.withheldAccount);
  }
  // dropped_count is a count PER GROUP, not one number: a picture that lost
  // three account contacts and none of ours is still partial, and summing is
  // the only honest way to say so in one sentence.
  const d = graph.dropped_count;
  const dropped = (d?.direct ?? 0) + (d?.account ?? 0) + (d?.peer ?? 0);
  if (dropped > 0) {
    parts.push(droppedText(dropped));
  }
  return parts.join(" ");
}

function colleagueNode(n: GraphNode, copy: MapCopy): MapNode {
  return {
    id: n.id,
    kind: "user",
    label: n.label,
    sublabel: n.sublabel,
    actions: [{ id: "use-route", label: copy.useThisRoute }],
  };
}

function contactNode(n: GraphNode, kind: "person"): MapNode {
  return { id: n.id, kind, label: n.label, sublabel: n.sublabel };
}

function labelOf(nodes: readonly GraphNode[], id: string): string {
  return nodes.find((n) => n.id === id)?.label ?? "";
}
