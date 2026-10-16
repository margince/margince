import type { RelationshipMapLabels } from "./relationshipmap";
import { type MapNode, type Placed, truncate } from "./relationshipmap.layout";

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
  // The accessible name carries what the drawing says in shape and thickness.
  // A reader who cannot see it gets the facts and not the picture.
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
        rx={placed.kind === "company" ? 12 : 8}
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
          className={`rmap-pill-${node.engagement ?? "untried"}`}
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
 * MoreRow is the tail of a capped lane. It shows how many contacts the lane
 * hides, as a control, because the reader's next move is to see them.
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
