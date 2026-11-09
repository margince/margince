import { extensions } from "@composition/extensions";
import type { Route } from "./router";

/**
 * The route as a pattern an observer may record: the screen, with each
 * record id replaced by a placeholder. A unit's screen keeps the unit's
 * name only when it is a composed unit. A fork screen is always `x/:key`,
 * because lib cannot see the fork registry. Any other id becomes a
 * placeholder, since anyone can type one into an address. Never carries the
 * query.
 */
export function routePattern(
  route: Route,
  unitNames: readonly string[] = extensions.map((unit) => unit.name),
): string {
  if (route.screen === "ext") {
    return route.id && unitNames.includes(route.id)
      ? `ext/${route.id}`
      : "ext/:unit";
  }
  if (route.screen === "x") {
    return "x/:key";
  }
  const parts: string[] = [route.screen];
  if (route.id) parts.push(":id");
  if (route.id2) parts.push(":id2");
  if (route.id3) parts.push(":id3");
  return parts.join("/");
}
