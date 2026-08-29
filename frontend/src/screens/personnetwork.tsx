// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The contact's network: who reaches them, how warmly, and what moved.
//
// This is the ONE person-graph surface. It reads `usePersonGraph`, leads with
// the warmest route because that is the answer, draws the ring as decoration
// over the node lists that are the content, and — where the caller hands it a
// 360 — closes with what changed lately. The old contacts screen renders it
// without that argument, so there is one component rather than two spellings
// of one question.

import { useState } from "react";

import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { Badge, Button, Card } from "../design-system/atoms";
import { SurfaceState } from "../design-system/surfacestate";
import { formatDate, formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { problemMessageOf } from "./common";
import { usePersonGraph } from "./persongraph";
import { changeSentence } from "./relationshipchange";
import "./personnetwork.css";

// What the moments card reads, and nothing more. Narrowed from Person360 so
// the old contacts screen — which holds no 360 — can still render the tab, and
// so a caller cannot be asked for fields this surface never touches.
type RelationshipMoments = Pick<
  components["schemas"]["Person360"],
  "relationship_changes" | "sections_omitted"
>;
type Graph = components["schemas"]["PersonGraph"];
type GraphNode = components["schemas"]["PersonGraphNode"];
type GraphEdge = components["schemas"]["PersonGraphEdge"];

// The ring's geometry, in the diagram's own coordinates. A square viewBox and
// a fixed radius: the picture is deterministic, so the same contact draws the
// same ring on every read and a rep learns where to look.
const VIEW = 260;
const CENTRE = 130;
const RING = 96;
const ANCHOR_RADIUS = 15;

// A node's radius by band. Strength is a size, not a colour, because colour on
// this ring already says which side of the account a node is on.
const RADIUS_BY_BAND: Readonly<Record<string, number>> = {
  strong: 11,
  moderate: 9,
  weak: 7,
  none: 6,
};

// Strongest first, so "the band of this node" has one answer when a node
// carries several edges.
const BAND_ORDER: readonly string[] = ["strong", "moderate", "weak", "none"];

type Placed = Readonly<{ node: GraphNode; x: number; y: number }>;

/**
 * ringLayout places the neighbours around the contact at the centre.
 *
 * Fixed, not force-directed, for the reason the account's own diagram gives:
 * the payload order is deterministic, so a fixed ring makes the picture
 * deterministic too. A force simulation would move every node whenever one
 * arrived, and a rep would never learn the shape.
 */
export function ringLayout(nodes: readonly GraphNode[]): Placed[] {
  const anchor = nodes.find((node) => node.group === "anchor");
  const ring = nodes.filter((node) => node.group !== "anchor");
  const placed: Placed[] = [];
  if (anchor) {
    placed.push({ node: anchor, x: CENTRE, y: CENTRE });
  }
  ring.forEach((node, index) => {
    // Start at twelve o'clock and go clockwise, so the first node in the
    // payload — the strongest — is the one at the top.
    const angle = (index / ring.length) * 2 * Math.PI - Math.PI / 2;
    placed.push({
      node,
      x: CENTRE + RING * Math.cos(angle),
      y: CENTRE + RING * Math.sin(angle),
    });
  });
  return placed;
}

/**
 * bandOf reports the STRONGEST band on any edge touching a node.
 *
 * Strongest rather than first: a node can carry several edges, and on the
 * account arm an edge need not touch the anchor at all. Sizing by whichever
 * edge the array happened to yield first would let a node's radius describe a
 * relationship the reader is not looking at.
 */
function bandOf(graph: Graph, nodeId: string): string {
  const bands = (graph.edges ?? [])
    .filter((edge) => edge.from === nodeId || edge.to === nodeId)
    .map((edge) => edge.strength_bucket);
  return (
    BAND_ORDER.find((band) => bands.some((held) => held === band)) ?? "none"
  );
}

/**
 * PersonNetworkTab answers "who reaches this contact, how warmly, and what
 * changed lately".
 *
 * `view` is optional because the moments come from the 360 and the old
 * contacts screen does not hold one. Without it the tab is everything else,
 * one card shorter — never a card claiming nothing moved.
 */
export function PersonNetworkTab({
  personId,
  view,
}: Readonly<{ personId: string; view?: RelationshipMoments }>) {
  const t = useT();
  const graph = usePersonGraph(personId);
  const [selected, setSelected] = useState<string | undefined>();

  if (graph.isPending) {
    return (
      <SurfaceState
        state="loading"
        emptyLabel={t("person.graph.noDirect")}
        loadingLabel={t("person.graph.loading")}
        loadingLines={3}
      >
        {null}
      </SurfaceState>
    );
  }
  if (graph.isError) {
    // Not SurfaceState's `failed`: that state drops its children for one
    // generic line, and WHICH failure this was is what a reader acts on — a
    // refusal is answered by asking for the grant, a timeout by retrying.
    // `problemMessageOf` is what keeps the internal cause off the screen.
    return (
      <p role="alert" className="pn-failed">
        {problemMessageOf(graph.error, t)}
      </p>
    );
  }
  // The arrays are required by the contract, and this guard is still load
  // bearing: a proxy error page or a version-skewed server delivers an object
  // without them, and rendering nothing beats taking the tab down.
  const data = graph.data;
  if (!data?.nodes) {
    return null;
  }
  const nodes = data.nodes;
  const anchor = nodes.find((n) => n.group === "anchor");
  const direct = nodes.filter((n) => n.group === "direct");
  const account = nodes.filter((n) => n.group === "account");
  const peers = nodes.filter((n) => n.group === "peer");
  // A selection only means something against the graph on screen. The tab
  // stays mounted as a reader moves between contacts, so a raw id would open
  // the detail region on a node this graph does not have.
  const selectedNode = nodes.some((n) => n.id === selected)
    ? selected
    : undefined;

  return (
    <div className="pn-stack">
      <RouteCard graph={data} />

      <Card
        title={t("person.network.ringTitle")}
        sub={t("person.network.ringSub")}
      >
        <div className="pn-split">
          <EgoRing graph={data} selected={selectedNode} />
          <div className="pn-groups">
            <NodeGroup
              title={t("person.graph.direct")}
              nodes={direct}
              selected={selectedNode}
              onSelect={setSelected}
              emptyLabel={t("person.graph.noDirect")}
              omitted={isOmitted(data, "direct")}
            />
            <NodeGroup
              title={t("person.graph.account")}
              nodes={account}
              selected={selectedNode}
              onSelect={setSelected}
              emptyLabel={t("person.graph.noAccount")}
              omitted={isOmitted(data, "account")}
            />
            <NodeGroup
              title={t("person.graph.peer")}
              nodes={peers}
              selected={selectedNode}
              onSelect={setSelected}
              emptyLabel={t("person.graph.noPeer")}
              omitted={isOmitted(data, "peer")}
            />
          </div>
        </div>
        {/* Selection changes this region without moving focus, so a reader on
            a screen reader would otherwise press a node and be told nothing
            happened. */}
        <div aria-live="polite" className="pn-live">
          {selectedNode && anchor && (
            <EdgeDetail
              graph={data}
              nodeId={selectedNode}
              anchorId={anchor.id}
            />
          )}
        </div>
        <DroppedNote graph={data} />
      </Card>

      {view && <MomentsCard view={view} />}
    </div>
  );
}

function isOmitted(graph: Graph, group: string): boolean {
  return (graph.groups_omitted ?? []).some((g) => g === group);
}

/**
 * RouteCard is the answer: the warmest way in and the evidence for it. It
 * leads because a reader who reads nothing else should still leave knowing
 * who to ask.
 */
function RouteCard({ graph }: Readonly<{ graph: Graph }>) {
  const t = useT();
  const route = graph.route;
  return (
    <Card title={t("person.graph.routeTitle")}>
      {route ? (
        <>
          <p className="pn-route">
            {route.through_display_name
              ? t("person.graph.routeVia", {
                  name: route.via_display_name,
                  through: route.through_display_name,
                })
              : t("person.graph.routeDirect", {
                  name: route.via_display_name,
                })}
          </p>
          <p className="pn-counts">{route.why}</p>
        </>
      ) : (
        <p className="pn-route">{t("person.graph.noRoute")}</p>
      )}
    </Card>
  );
}

/**
 * EgoRing draws the contact at the centre with everyone who reaches them on
 * the ring, edge weight carrying the server's band.
 *
 * Decorative: `aria-hidden`, nothing focusable, no click target. Every node in
 * it is a button in the list beside it.
 */
function EgoRing({
  graph,
  selected,
}: Readonly<{ graph: Graph; selected: string | undefined }>) {
  const placed = ringLayout(graph.nodes ?? []);
  const at = new Map(placed.map((p) => [p.node.id, p]));
  // A ring with nobody on it is one dot, which draws as a smudge rather than a
  // picture. The lists beside it already say nobody reaches this contact.
  if (placed.length < 2) {
    return null;
  }
  return (
    <svg
      className="pn-ring"
      viewBox={`0 0 ${VIEW} ${VIEW}`}
      aria-hidden="true"
      focusable="false"
    >
      {(graph.edges ?? []).map((edge) => {
        const from = at.get(edge.from);
        const to = at.get(edge.to);
        // The server guarantees both ends are nodes. A payload where they are
        // not is one this build does not understand, and a line to nowhere
        // draws as a stray stroke across the canvas.
        if (!from || !to) {
          return null;
        }
        const active = selected === edge.from || selected === edge.to;
        return (
          <line
            key={`${edge.from}->${edge.to}`}
            className={edgeClass(edge, active)}
            x1={from.x}
            y1={from.y}
            x2={to.x}
            y2={to.y}
          />
        );
      })}
      {placed.map((p) => (
        <circle
          key={p.node.id}
          className={nodeClass(p.node, selected === p.node.id)}
          cx={p.x}
          cy={p.y}
          r={
            p.node.group === "anchor"
              ? ANCHOR_RADIUS
              : (RADIUS_BY_BAND[bandOf(graph, p.node.id)] ??
                RADIUS_BY_BAND.none)
          }
        />
      ))}
    </svg>
  );
}

function edgeClass(edge: GraphEdge, active: boolean): string {
  const weight = `pn-edge pn-edge-${edge.strength_bucket}`;
  return active ? `${weight} pn-edge-active` : weight;
}

function nodeClass(node: GraphNode, selected: boolean): string {
  const kind =
    node.group === "anchor"
      ? "pn-node-anchor"
      : node.type === "colleague"
        ? "pn-node-colleague"
        : "pn-node-contact";
  return selected ? `pn-node ${kind} pn-node-selected` : `pn-node ${kind}`;
}

/**
 * NodeGroup renders one arm of the ring as real buttons with aria-pressed.
 * The list is the content; the ring is the picture of it.
 *
 * SurfaceState owns the withheld-or-empty decision, because those two are the
 * pair a reader must never see merged: a withheld arm arrives with no nodes,
 * and saying "nobody here knows them" beside "you cannot see this" states an
 * absence the server never claimed.
 */
function NodeGroup({
  title,
  nodes,
  selected,
  onSelect,
  emptyLabel,
  omitted,
}: Readonly<{
  title: string;
  nodes: GraphNode[];
  selected: string | undefined;
  onSelect: (id: string) => void;
  emptyLabel: string;
  omitted: boolean;
}>) {
  const state = omitted
    ? ("withheld" as const)
    : nodes.length === 0
      ? ("empty" as const)
      : ("ready" as const);
  return (
    <SurfaceState label={title} state={state} emptyLabel={emptyLabel}>
      <ul className="pn-chips">
        {nodes.map((node) => (
          <li key={node.id}>
            <Button
              small
              aria-pressed={selected === node.id}
              onClick={() => onSelect(node.id)}
            >
              {node.label}
              {node.sublabel ? ` · ${node.sublabel}` : ""}
            </Button>
          </li>
        ))}
      </ul>
    </SurfaceState>
  );
}

/**
 * EdgeDetail shows what the selected node's relationship is made of.
 *
 * The receipts are the point: counts say a route exists, the messages say the
 * reader is not being asked to trust a number.
 */
function EdgeDetail({
  graph,
  nodeId,
  anchorId,
}: Readonly<{ graph: Graph; nodeId: string; anchorId: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const node = graph.nodes?.find((n) => n.id === nodeId);
  const edges = (graph.edges ?? []).filter(
    (e) => e.from === nodeId || e.to === nodeId,
  );
  if (!node || edges.length === 0) {
    return (
      <p className="pn-counts">
        {t("person.graph.noEdge", { name: node?.label ?? "" })}
      </p>
    );
  }
  return (
    <Card title={node.label} level={3}>
      {edges.map((edge) => {
        // Whom this edge joins the SELECTED node to. Both ends are read
        // because an account-arm edge need not touch the anchor at all.
        const otherEnd = edge.from === nodeId ? edge.to : edge.from;
        const otherNode = graph.nodes?.find((n) => n.id === otherEnd);
        const withWhom =
          otherEnd === anchorId
            ? t("person.graph.withContact")
            : otherNode
              ? t("person.graph.withColleague", { name: otherNode.label })
              : undefined;
        const receipts = edge.receipts ?? [];
        return (
          <div key={`${edge.from}->${edge.to}`} className="pn-edge-facts">
            <p className="pn-band">
              <Badge>{t(`person.band.${edge.strength_bucket}`)}</Badge>
              {withWhom && <span>{withWhom}</span>}
            </p>
            <p className="pn-counts">
              {t("person.graph.counts", {
                total: formatNumber(edge.interactions_90d, locale),
                inbound: formatNumber(edge.inbound_90d ?? 0, locale),
                outbound: formatNumber(edge.outbound_90d ?? 0, locale),
              })}
            </p>
            {receipts.length > 0 ? (
              <ul className="pn-receipts">
                {receipts.map((r) => (
                  <li key={r.activity_id}>
                    {r.subject ?? t("person.graph.untitledMessage")} ·{" "}
                    {/* The record's zone: a receipt is evidence of when a
                        message happened, so every reader names the same day. */}
                    {formatDate(r.occurred_at, locale, recordZone)}
                  </li>
                ))}
              </ul>
            ) : (
              <p className="pn-counts">{t("person.graph.countsOnly")}</p>
            )}
          </div>
        );
      })}
    </Card>
  );
}

/**
 * MomentsCard is what changed about this relationship lately.
 *
 * It is the difference between a picture of a network and a live one: a ring
 * says who is there, a moment says what moved. The sentences are the 360's
 * own, so one derived change does not get two sets of words.
 */
function MomentsCard({ view }: Readonly<{ view: RelationshipMoments }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  const changes = view.relationship_changes ?? [];
  // The section is withholdable. A reader without the grant is served no
  // changes, and "nothing has moved" would be a fact the page does not have.
  const withheld = (view.sections_omitted ?? []).some(
    (section) => section === "relationship_changes",
  );
  const state = withheld
    ? ("withheld" as const)
    : changes.length === 0
      ? ("empty" as const)
      : ("ready" as const);
  return (
    <Card
      title={t("person.network.momentsTitle")}
      sub={t("person.network.momentsSub")}
    >
      <SurfaceState state={state} emptyLabel={t("person.network.noMoments")}>
        <ul className="pn-moments">
          {changes.map((change) => (
            <li key={`${change.kind}-${change.at}`} className="pn-moment">
              <span>{changeSentence(change, t)}</span>
              <span className="pn-moment-when">
                {formatDate(change.at, locale, recordZone)}
              </span>
            </li>
          ))}
        </ul>
      </SurfaceState>
    </Card>
  );
}

/**
 * DroppedNote says what the ring is NOT showing. Silent truncation reads as
 * "this is everyone", which is the one thing a graph must never imply falsely.
 */
function DroppedNote({ graph }: Readonly<{ graph: Graph }>) {
  const t = useT();
  const { locale } = useLocale();
  const dropped =
    (graph.dropped_count?.direct ?? 0) +
    (graph.dropped_count?.account ?? 0) +
    (graph.dropped_count?.peer ?? 0);
  if (dropped === 0) {
    return null;
  }
  return (
    <p className="pn-note">
      {t("person.graph.dropped", { count: formatNumber(dropped, locale) })}
    </p>
  );
}
