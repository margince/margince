import { Button } from "./atoms";
import { Heading } from "./heading";
import type { RelationshipMapLabels } from "./relationshipmap";
import type {
  MapNode,
  RelationshipMapModel,
  routeFor,
} from "./relationshipmap.layout";

/**
 * Panel says in words what the highlight says in the picture. It also lists
 * the routes the highlight did not take, so a reader can disagree with the
 * recommendation rather than only accept it.
 */
export function Panel({
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
  // A route runs colleague → contact, so which END is the reader's depends on
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
      <Heading size="medium" className="rmap-panel-title">
        {node.label}
      </Heading>
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
