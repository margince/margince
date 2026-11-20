import type { RelationshipMapLabels } from "./relationshipmap";
import {
  fitLabel,
  type MapNode,
  NAME_CHARS,
  NODE_INSET,
  type Placed,
} from "./relationshipmap.layout";
import type { MapTextMeasures } from "./relationshipmap.measure";

const SUB_CHARS = 26;

export function Node({
  placed,
  node,
  labels,
  lane,
  focused,
  related,
  faded,
  tabbable,
  onPress,
  measures,
}: Readonly<{
  placed: Placed;
  node?: MapNode;
  labels: RelationshipMapLabels;
  measures: MapTextMeasures;
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
        measures={measures}
      />
    );
  }
  if (!node) {
    return null;
  }
  // The accessible name carries what the drawing says in shape and thickness.
  // A reader who cannot see it gets the facts and not the picture.
  const name = [node.label, node.sublabel, lane?.label, node.engagementLabel]
    .filter(Boolean)
    .join(", ");
  const room = placed.w - NODE_INSET * 2;
  const textX = placed.x + NODE_INSET;
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
        rx={placed.kind === "company" ? 12 : 8}
      />
      <text className="rmap-name" x={textX} y={placed.y + 22}>
        {fitLabel(node.label, room, measures.name, NAME_CHARS)}
      </text>
      {node.sublabel && (
        <text className="rmap-sub" x={textX} y={placed.y + 38}>
          {fitLabel(node.sublabel, room, measures.sub, SUB_CHARS)}
        </text>
      )}
      {node.engagementLabel && (
        <text
          className={`rmap-pill-${node.engagement ?? "untried"}`}
          x={textX}
          y={placed.y + 54}
        >
          {fitLabel(node.engagementLabel, room, measures.pill)}
        </text>
      )}
    </g>
  );
}

/**
 * MoreRow is the tail of a capped lane. It shows how many contacts the lane
 * hides, as a control, because the reader's next move is to see them.
 */
function MoreRow({
  placed,
  labels,
  hidden,
  tabbable,
  onPress,
  measures,
}: Readonly<{
  placed: Placed;
  labels: RelationshipMapLabels;
  hidden: number;
  tabbable: boolean;
  onPress: (id: string) => void;
  measures: MapTextMeasures;
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
      <text x={placed.x + NODE_INSET} y={placed.y + 19}>
        {fitLabel(
          labels.laneMore(hidden),
          placed.w - NODE_INSET * 2,
          measures.more,
        )}
      </text>
    </g>
  );
}
