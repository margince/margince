import { type ReactNode, useEffect, useMemo, useRef, useState } from "react";
import { usePrefersReducedMotion } from "./motion";
import {
  type Layout,
  layout,
  type MapEdge,
  type RelationshipMapModel,
  routeFor,
} from "./relationshipmap.layout";
import "./relationshipmap.css";

import { Panel } from "./relationshipmappanel";
// The account's routes, drawn.
//
// It shows what a list cannot: which colleague can reach which contact, how
// warm each route is, and where the buying team has a hole.
//
// Data-only, and every string arrives translated. A primitive that reached for
// the catalog would decide copy for screens it has never seen, and its stories
// could not render without a locale.
import { Node } from "./relationshipnode";

export type RelationshipMapLabels = Readonly<{
  /** Names the whole picture for a reader who cannot see it. */
  region: string;
  band: Record<"strong" | "developing" | "cold", string>;
  bestRoute: string;
  alternatives: string;
  noRoute: string;
  laneMore: (hidden: number) => string;
  clearFocus: string;
  emptyTitle: string;
  emptyBody: string;
  nothingSelected: string;
}>;

export function RelationshipMap({
  model,
  focusId,
  onFocus,
  onAction,
  completenessText,
  labels,
  reducedMotion,
  panelSlot,
}: Readonly<{
  model: RelationshipMapModel;
  focusId: string | null;
  onFocus: (id: string | null) => void;
  onAction?: (nodeId: string, actionId: string) => void;
  completenessText: string;
  labels: RelationshipMapLabels;
  /** Overrides the media query, so a story can show the still version. */
  reducedMotion?: boolean;
  panelSlot?: ReactNode;
}>) {
  const [expanded, setExpanded] = useState<ReadonlySet<string>>(new Set());
  const prefersReduced = usePrefersReducedMotion();
  const still = reducedMotion ?? prefersReduced;
  const placement = useMemo(() => layout(model, expanded), [model, expanded]);
  const { related, route } = useMemo(
    () => routeFor(model, focusId),
    [model, focusId],
  );
  const byId = useMemo(
    () => new Map(model.nodes.map((node) => [node.id, node])),
    [model],
  );
  const litEdges = new Set(route?.edgeIds ?? []);
  // A focus the model no longer holds fades NOTHING. routeFor already answers
  // "nothing is related" for a stale id, and fading on the raw prop instead
  // would push the whole account back for a selection that is not there.
  const dimming = focusId !== null && related.size > 0;

  // One tab stop for the whole picture, then arrow keys walk it. Thirty nodes
  // at thirty tab stops would be a reason to skip the map, not a way through it.
  //
  // The cursor is an ID, not an index. An index into the placed list is stale
  // the moment a lane expands, and one that survives into a different list
  // silently points at somebody else.
  const order = placement.placed.map((node) => node.id);
  const [cursorId, setCursorId] = useState<string | null>(null);
  const focused = cursorId && order.includes(cursorId) ? cursorId : order[0];
  const svgRef = useRef<SVGSVGElement>(null);
  // What the cursor was when this render drew, so the effect below can tell a
  // keyboard move from a re-render and only chase the former.
  const moved = useRef(false);

  // The selection follows the caller: a focus set from outside (a row pressed
  // in the list beside this map) puts the cursor on that node. It does NOT run
  // on every render — `order` is a new array each time, and an effect keyed on
  // it snapped the cursor back to the selection on every arrow press, so a
  // reader could never walk away from what was selected.
  useEffect(() => {
    if (focusId) {
      setCursorId(focusId);
    }
  }, [focusId]);

  // Focus FOLLOWS the cursor. Moving only the tabindex leaves the ring, the
  // screen reader and Enter all pointing at the node the reader walked away
  // from — the arrow keys look like they work and activate the wrong contact.
  useEffect(() => {
    if (!moved.current || !focused) {
      return;
    }
    moved.current = false;
    const node = svgRef.current?.querySelector<SVGGElement>(
      `[data-node-id="${CSS.escape(focused)}"]`,
    );
    node?.focus();
  }, [focused]);

  if (model.nodes.length === 0) {
    return (
      <div className="rmap-empty">
        <p className="rmap-empty-title">{labels.emptyTitle}</p>
        <p className="rmap-empty-body">{labels.emptyBody}</p>
      </div>
    );
  }

  const openLane = (id: string) => {
    const lane = id.slice("more:".length);
    // The row the reader stands on is replaced by the contacts it hid, so the
    // cursor moves to the first of them, or focus falls out of the map.
    const firstHidden = model.lanes
      .find((candidate) => candidate.id === lane)
      ?.nodeIds.find((nodeId) => !order.includes(nodeId));
    setExpanded((prev) => new Set([...prev, lane]));
    if (firstHidden) {
      moved.current = true;
      setCursorId(firstHidden);
    }
  };

  const press = (id: string) => {
    if (id.startsWith("more:")) {
      openLane(id);
      return;
    }
    // Pressing the focused node again clears it: the way out is the way in,
    // rather than a control the reader has to find.
    onFocus(focusId === id ? null : id);
  };

  const onKeyDown = (event: React.KeyboardEvent) => {
    if (event.key === "Escape") {
      onFocus(null);
      return;
    }
    const step = stepFor(event.key);
    if (step !== 0) {
      event.preventDefault();
      const at = focused ? order.indexOf(focused) : 0;
      const next = Math.min(Math.max(at + step, 0), order.length - 1);
      moved.current = true;
      setCursorId(order[next] ?? null);
      return;
    }
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      if (focused !== undefined) {
        press(focused);
      }
    }
  };

  return (
    <div className="rmap-frame">
      <div className="rmap" data-motion={still ? "none" : "in"}>
        <div className="rmap-scroll">
          {/* biome-ignore lint/a11y/useSemanticElements: the rule suggests
            <fieldset>, which cannot live inside an SVG. role="group" with a
            label is how an SVG names itself to a reader who cannot see it. */}
          <svg
            ref={svgRef}
            className="rmap-svg"
            viewBox={`0 0 ${placement.width} ${placement.height}`}
            width={placement.width}
            height={placement.height}
            role="group"
            aria-label={labels.region}
            onKeyDown={onKeyDown}
          >
            <title>{labels.region}</title>
            {/* One hidden layer rather than a hidden attribute per line: a <g>
              is not focusable in any engine, so the intent is unambiguous, and
              the edges' facts reach a reader through the node names and the
              panel instead. */}
            {/* biome-ignore lint/a11y/noAriaHiddenOnFocusable: a <g> is not a
              focusable element; the rule flags any aria-hidden inside an SVG.
              Hiding the edge layer is correct rather than lossy — what a line
              says in thickness and dash reaches a reader through the node
              accessible names and the panel. */}
            <g aria-hidden="true">
              {model.edges.map((edge) => (
                <Edge
                  key={edge.id}
                  edge={edge}
                  placement={placement}
                  lit={litEdges.has(edge.id)}
                  faded={dimming && !litEdges.has(edge.id)}
                />
              ))}
            </g>
            {placement.heads.map((head) => (
              <text
                key={head.id}
                className="rmap-lane t-eyebrow"
                x={head.x}
                y={head.y + 12}
              >
                {head.label}
              </text>
            ))}
            {placement.placed.map((node) => (
              <Node
                key={node.id}
                placed={node}
                node={byId.get(node.id)}
                labels={labels}
                lane={placement.heads.find((head) => head.id === node.laneId)}
                focused={focusId === node.id}
                related={related.has(node.id)}
                faded={dimming && !related.has(node.id)}
                tabbable={node.id === focused}
                onPress={press}
              />
            ))}
          </svg>
        </div>
        <p className="rmap-completeness">{completenessText}</p>
        <div className="rmap-panel">
          {panelSlot}
          <Panel
            model={model}
            focusId={focusId}
            node={focusId ? byId.get(focusId) : undefined}
            route={route}
            labels={labels}
            onAction={onAction}
          />
        </div>
      </div>
    </div>
  );
}

/**
 * stepFor is which way an arrow walks the map.
 *
 * Both axes move along the same one-dimensional order rather than in the
 * direction they point: the walk is lane by lane, and a reader pressing right
 * at the end of a lane wants the next lane, not nothing.
 */
function stepFor(key: string): number {
  if (key === "ArrowDown" || key === "ArrowRight") {
    return 1;
  }
  if (key === "ArrowUp" || key === "ArrowLeft") {
    return -1;
  }
  return 0;
}

function Edge({
  edge,
  placement,
  lit,
  faded,
}: Readonly<{
  edge: MapEdge;
  placement: Layout;
  lit: boolean;
  faded: boolean;
}>) {
  const from = placement.placed.find((node) => node.id === edge.from);
  const to = placement.placed.find((node) => node.id === edge.to);
  if (!from || !to) {
    // An edge naming a node this lane capped away is not drawn. Drawing it to
    // nowhere would be a line the reader cannot follow.
    return null;
  }
  const x1 = from.x + from.w;
  const y1 = from.y + from.h / 2;
  const x2 = to.x;
  const y2 = to.y + to.h / 2;
  const bend = 48;
  return (
    <path
      className={`rmap-edge rmap-edge-${edge.kind}${edge.band ? ` rmap-band-${edge.band}` : ""}`}
      data-lit={lit ? "true" : undefined}
      data-faded={faded ? "true" : undefined}
      d={`M ${x1} ${y1} C ${x1 + bend} ${y1} ${x2 - bend} ${y2} ${x2} ${y2}`}
      focusable="false"
    />
  );
}
