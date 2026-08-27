import { useState } from "react";

import type { components } from "../api/schema";
import { useRecordZone } from "../app/recordzone";
import { Badge, Button, Card, SectionHeader } from "../design-system/atoms";
import { formatDate, formatNumber } from "../format/format";
import { useLocale, useT } from "../i18n";
import { problemMessageOf } from "./common";
import { usePersonGraph } from "./persongraph";
import "./personnetwork.css";

type Person360 = components["schemas"]["Person360"];
type Graph = components["schemas"]["PersonGraph"];
type GraphNode = components["schemas"]["PersonGraphNode"];
type GraphEdge = components["schemas"]["PersonGraphEdge"];
type Change = components["schemas"]["PersonRelationshipChange"];

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

type Placed = Readonly<{ node: GraphNode; x: number; y: number }>;

/**
 * ringLayout places the neighbours around the contact at the centre.
 *
 * Fixed, not force-directed, for the reason the account's own diagram gives:
 * the payload order is deterministic, so a fixed ring makes the picture
 * deterministic too. A force simulation would move every node whenever one
 * arrived, and a rep would never learn the shape.
 *
 * Colleagues occupy the first half of the ring and account contacts the
 * second, so our side and their side are two arcs a reader can tell apart
 * without reading a single label.
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

/** bandOf reports the server's band for a node, from the edge that carries it. */
function bandOf(graph: Graph, nodeId: string): string {
  const edge = (graph.edges ?? []).find(
    (e) => e.from === nodeId || e.to === nodeId,
  );
  return edge?.strength_bucket ?? "none";
}

/**
 * PersonNetworkTab answers "who reaches this contact, how warmly, and what
 * changed lately".
 *
 * The warmest route leads because it is the answer. The ring is decoration
 * over the node lists, which are the content: the same nodes, in the same
 * order, as real buttons. Nothing in the picture is reachable only by
 * pointing at it.
 */
export function PersonNetworkTab({
  personId,
  view,
}: Readonly<{ personId: string; view: Person360 }>) {
  const t = useT();
  const graph = usePersonGraph(personId);
  const [selected, setSelected] = useState<string | undefined>();

  if (graph.isPending) {
    return <p>{t("person.graph.loading")}</p>;
  }
  if (graph.isError) {
    return <p role="alert">{problemMessageOf(graph.error, t)}</p>;
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
  // A selection only means something against the graph on screen. The tab
  // stays mounted as a reader moves between contacts, so a raw id would open
  // the detail region on a node this graph does not have.
  const selectedNode = nodes.some((n) => n.id === selected)
    ? selected
    : undefined;

  return (
    <div className="pn-stack">
      <RouteCard graph={data} />

      <Card>
        <div style={{ padding: "var(--space-4)" }}>
          <SectionHeader
            title={t("person.network.ringTitle")}
            sub={t("person.network.ringSub")}
          />
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
            </div>
          </div>
          {/* Selection changes this region without moving focus, so a reader
              on a screen reader would otherwise press a node and be told
              nothing happened. */}
          <div aria-live="polite" className="pn-detail">
            {selectedNode && anchor && (
              <EdgeDetail
                graph={data}
                nodeId={selectedNode}
                anchorId={anchor.id}
              />
            )}
          </div>
          <DroppedNote graph={data} />
        </div>
      </Card>

      <MomentsCard view={view} />
    </div>
  );
}

function isOmitted(graph: Graph, group: string): boolean {
  return (graph.groups_omitted ?? []).some((g) => g === group);
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
 * RouteCard is the answer: the warmest way in and the evidence for it. It
 * leads because a reader who reads nothing else should still leave knowing
 * who to ask.
 */
function RouteCard({ graph }: Readonly<{ graph: Graph }>) {
  const t = useT();
  const route = graph.route;
  return (
    <Card>
      <div style={{ padding: "var(--space-4)" }}>
        <SectionHeader title={t("person.graph.routeTitle")} />
        {route ? (
          <>
            <p style={{ margin: "var(--space-2) 0 0", lineHeight: 1.5 }}>
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
          <p style={{ margin: "var(--space-2) 0 0", lineHeight: 1.5 }}>
            {t("person.graph.noRoute")}
          </p>
        )}
      </div>
    </Card>
  );
}

/**
 * NodeGroup renders one arm of the ring as real buttons with aria-pressed.
 * The list is the content; the ring is the picture of it.
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
  const t = useT();
  return (
    <div>
      <SectionHeader title={title} />
      {nodes.length === 0 ? (
        <p className="pn-empty">{emptyLabel}</p>
      ) : (
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
      )}
      {/* Withheld is not empty. A group the reader may not see says so, rather
          than rendering the sentence an absence would have produced. */}
      {omitted && <p className="pn-empty">{t("person.graph.omitted")}</p>}
    </div>
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
      <p className="pn-empty">
        {t("person.graph.noEdge", { name: node?.label ?? "" })}
      </p>
    );
  }
  return (
    <div>
      <SectionHeader title={node.label} />
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
          <div key={`${edge.from}->${edge.to}`} className="pn-detail">
            <p
              style={{
                margin: 0,
                display: "flex",
                gap: "var(--space-2)",
                alignItems: "center",
              }}
            >
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
    </div>
  );
}

/**
 * MomentsCard is what changed about this relationship lately.
 *
 * It is the difference between a picture of a network and a live one: a ring
 * says who is there, a moment says what moved. The changes are derived at
 * read from the same curve the bands come from, so this card needs no state
 * of its own.
 */
function MomentsCard({ view }: Readonly<{ view: Person360 }>) {
  const t = useT();
  const changes = view.relationship_changes ?? [];
  return (
    <Card>
      <div style={{ padding: "var(--space-4)" }}>
        <SectionHeader
          title={t("person.network.momentsTitle")}
          sub={t("person.network.momentsSub")}
        />
        {changes.length === 0 ? (
          <p className="pn-empty">{t("person.network.noMoments")}</p>
        ) : (
          <ul className="pn-moments">
            {changes.map((change) => (
              <Moment key={`${change.kind}-${change.at}`} change={change} />
            ))}
          </ul>
        )}
      </div>
    </Card>
  );
}

function Moment({ change }: Readonly<{ change: Change }>) {
  const t = useT();
  const { locale } = useLocale();
  const recordZone = useRecordZone();
  return (
    <li className="pn-moment">
      <span>
        {change.days === undefined
          ? t(`person.network.moment.${change.kind}`)
          : t(`person.network.momentDays.${change.kind}`, {
              days: formatNumber(change.days, locale),
            })}
      </span>
      <span className="pn-moment-when">
        {formatDate(change.at, locale, recordZone)}
      </span>
    </li>
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
    (graph.dropped_count?.direct ?? 0) + (graph.dropped_count?.account ?? 0);
  if (dropped === 0) {
    return null;
  }
  return (
    <p className="pn-note">
      {t("person.graph.dropped", { count: formatNumber(dropped, locale) })}
    </p>
  );
}
