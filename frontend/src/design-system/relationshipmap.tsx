import { type ReactNode, useEffect, useMemo, useRef, useState } from "react";
import { Button } from "./atoms";
import { usePrefersReducedMotion } from "./motion";
import {
  type Layout,
  layout,
  type MapEdge,
  type MapNode,
  type Placed,
  type RelationshipMapModel,
  routeFor,
  truncate,
} from "./relationshipmap.layout";
import "./relationshipmap.css";

// The account's routes, drawn.
//
// A picture earns its place here by showing what a list cannot: which of our
// colleagues can reach which of their people, how warm each of those routes is,
// and where the buying team has a hole. The previous diagram on this page was
// unlabeled dots hidden from screen readers — it showed that the account had
// many connections, which the page already said in words.
//
// Data-only, and every string arrives translated. A primitive that reached for
// the catalog would decide copy for screens it has never seen, and its stories
// could not render without a locale.

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

  // One tab stop for the whole picture, then the arrow keys walk it. A map of
  // thirty nodes that took thirty tab stops would be a reason to skip the tab
  // rather than a way through it.
  //
  // The cursor is an ID, not an index. An index into the placed list is stale
  // the moment a lane expands — the row the reader was standing on is replaced
  // by the people it was hiding — and an index that survives into a different
  // list silently points at somebody else.
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
  // from — the arrow keys look like they work and activate the wrong person.
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
    // The row the reader is standing on is about to be replaced by the people
    // it was hiding, so the cursor moves to the first of them — otherwise
    // focus falls out of the map entirely.
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
      press(focused);
    }
  };

  return (
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
      className={`rmap-edge rmap-edge-${edge.kind} rmap-band-${edge.band ?? "none"}`}
      data-lit={lit ? "true" : undefined}
      data-faded={faded ? "true" : undefined}
      d={`M ${x1} ${y1} C ${x1 + bend} ${y1} ${x2 - bend} ${y2} ${x2} ${y2}`}
      focusable="false"
    />
  );
}

function Node({
  placed,
  node,
  labels,
  lane,
  focused,
  related,
  faded,
  tabbable,
  onPress,
}: Readonly<{
  placed: Placed;
  node?: MapNode;
  labels: RelationshipMapLabels;
  lane?: { label: string; hidden: number };
  focused: boolean;
  related: boolean;
  faded: boolean;
  tabbable: boolean;
  onPress: (id: string) => void;
}>) {
  if (placed.kind === "more") {
    return (
      <MoreRow
        placed={placed}
        labels={labels}
        hidden={lane?.hidden ?? 0}
        tabbable={tabbable}
        onPress={onPress}
      />
    );
  }
  if (!node) {
    return null;
  }
  // The accessible name carries what the drawing says in shape and thickness,
  // because a reader who cannot see it gets the facts and not the picture.
  const name = [node.label, node.sublabel, lane?.label, node.engagementLabel]
    .filter(Boolean)
    .join(", ");
  return (
    // biome-ignore lint/a11y/useSemanticElements: a <button> cannot be an SVG child; the node carries the role, label, pressed state and tab stop instead.
    <g
      className={`rmap-node rmap-${placed.kind}`}
      role="button"
      tabIndex={tabbable ? 0 : -1}
      aria-pressed={focused}
      aria-label={name}
      data-node-id={placed.id}
      data-kind={placed.kind}
      data-faded={faded ? "true" : undefined}
      data-related={related && !focused ? "true" : undefined}
      data-focused={focused ? "true" : undefined}
      data-added={node.addedBySearch ? "true" : undefined}
      onClick={() => onPress(placed.id)}
    >
      <title>{name}</title>
      <rect
        className="rmap-box"
        x={placed.x}
        y={placed.y}
        width={placed.w}
        height={placed.h}
        rx={placed.kind === "organization" ? 12 : 8}
      />
      <text className="rmap-name" x={placed.x + 12} y={placed.y + 22}>
        {truncate(node.label)}
      </text>
      {node.sublabel && (
        <text className="rmap-sub" x={placed.x + 12} y={placed.y + 38}>
          {truncate(node.sublabel, 26)}
        </text>
      )}
      {node.engagementLabel && (
        <text
          className={`rmap-pill rmap-pill-${node.engagement ?? "untried"}`}
          x={placed.x + 12}
          y={placed.y + 54}
        >
          {node.engagementLabel}
        </text>
      )}
    </g>
  );
}

/**
 * Panel says in words what the highlight says in the picture, and lists the
 * routes the highlight did NOT take — which is how a reader can disagree with
 * the recommendation rather than only accept it.
 */
function Panel({
  model,
  focusId,
  node,
  route,
  labels,
  onAction,
}: Readonly<{
  model: RelationshipMapModel;
  focusId: string | null;
  node?: MapNode;
  route: ReturnType<typeof routeFor>["route"];
  labels: RelationshipMapLabels;
  onAction?: (nodeId: string, actionId: string) => void;
}>) {
  if (!focusId || !node) {
    return <p className="rmap-hint">{labels.nothingSelected}</p>;
  }
  const best = route
    ? model.edges.find((e) => e.id === route.edgeIds[0])
    : null;
  // A route runs colleague → person, so which END is the reader's depends on
  // which they selected. Reading it one way round printed "Lars Meyer → Lars
  // Meyer" for a colleague focus and listed none of their other routes.
  const fromColleague = node.kind === "user";
  const others = model.edges.filter(
    (edge) =>
      edge.kind === "route" &&
      (fromColleague ? edge.from === focusId : edge.to === focusId) &&
      edge.id !== best?.id,
  );
  const nameOf = (id: string) =>
    model.nodes.find((candidate) => candidate.id === id)?.label ?? id;
  return (
    <div>
      <h3 className="rmap-panel-title">{node.label}</h3>
      {node.sublabel && <p className="rmap-panel-sub">{node.sublabel}</p>}
      <p className="rmap-panel-label t-eyebrow">{labels.bestRoute}</p>
      {best ? (
        <p className="rmap-panel-line">
          {fromColleague ? node.label : nameOf(best.from)} →{" "}
          {fromColleague ? nameOf(best.to) : node.label} ·{" "}
          {labels.band[best.band ?? "cold"]} · {best.words}
        </p>
      ) : (
        <p className="rmap-panel-line">{labels.noRoute}</p>
      )}
      {others.length > 0 && (
        <>
          <p className="rmap-panel-label t-eyebrow">{labels.alternatives}</p>
          {others.map((edge) => (
            <p key={edge.id} className="rmap-panel-alt">
              {nameOf(fromColleague ? edge.to : edge.from)} ·{" "}
              {labels.band[edge.band ?? "cold"]} · {edge.words}
            </p>
          ))}
        </>
      )}
      {node.actions && node.actions.length > 0 && (
        <div className="rmap-panel-actions">
          {node.actions.map((action) => (
            <Button
              key={action.id}
              variant={action.primary ? "primary" : "ghost"}
              small
              onClick={() => onAction?.(node.id, action.id)}
            >
              {action.label}
            </Button>
          ))}
        </div>
      )}
    </div>
  );
}

/**
 * MoreRow is the tail of a capped lane: the count of who is not drawn, as a
 * control rather than a caption, because the reader's next move is to see them.
 */
function MoreRow({
  placed,
  labels,
  hidden,
  tabbable,
  onPress,
}: Readonly<{
  placed: Placed;
  labels: RelationshipMapLabels;
  hidden: number;
  tabbable: boolean;
  onPress: (id: string) => void;
}>) {
  return (
    // biome-ignore lint/a11y/useSemanticElements: a <button> cannot be an SVG child; the role and tab stop carry what it would have given.
    <g
      className="rmap-node rmap-more"
      role="button"
      tabIndex={tabbable ? 0 : -1}
      aria-label={labels.laneMore(hidden)}
      data-node-id={placed.id}
      onClick={() => onPress(placed.id)}
    >
      <rect
        x={placed.x}
        y={placed.y}
        width={placed.w}
        height={placed.h}
        rx={8}
      />
      <text x={placed.x + 12} y={placed.y + 19}>
        {labels.laneMore(hidden)}
      </text>
    </g>
  );
}
