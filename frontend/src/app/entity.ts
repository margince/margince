import type { Route } from "./router";

// The one place the app's record kinds are enumerated. The history endpoints,
// EntityRef, and LogActivity all speak this vocabulary; before this registry
// each kept its own contact|company|deal union (all missing lead).
// `activity` is intentionally absent: it is the timeline, not a 360 record.
export type EntityKind = "contact" | "company" | "deal" | "lead" | "project";

export const ENTITY_KINDS = [
  "contact",
  "company",
  "deal",
  "lead",
  "project",
] as const satisfies readonly EntityKind[];

const ROUTABLE_KINDS: ReadonlySet<string> = new Set(ENTITY_KINDS);

// Whether a kind that arrived as free-form wire text is one this app can route
// to. Activity links and audit rows both carry kinds beyond the routable set
// (`activity` itself, and the governance objects an audit row names), so a
// caller that wants to LINK a record has to ask first.
export function isEntityKind(kind: string): kind is EntityKind {
  return ROUTABLE_KINDS.has(kind);
}

export type EntityDescriptor = {
  route: (id: string) => Route;
};

export const ENTITY: Record<EntityKind, EntityDescriptor> = {
  contact: {
    route: (id) => ({ screen: "contacts", id }),
  },
  company: {
    route: (id) => ({ screen: "companies", id }),
  },
  deal: {
    route: (id) => ({ screen: "deals", id }),
  },
  lead: {
    route: (id) => ({ screen: "leads", id }),
  },
  project: {
    route: (id) => ({ screen: "projects", id }),
  },
};

/**
 * The record page a typed reference names, or undefined when there is none.
 *
 * Every surface that carries a `{type, id}` off the wire — an approval's undo
 * target, a worklist row's subject, a notification's target — asks the same two
 * questions before it offers a link, so they ask them HERE rather than each
 * spelling `isEntityKind` and then `ENTITY[...].route` for itself. It lives
 * beside the registry it is built from: kept in a screen file, it was reachable
 * only by whoever already knew which screen, and a new record kind would have
 * arrived in one copy of the rule and not the others.
 *
 * A kind with no page — `activity`, an audit row's governance object — resolves
 * to nothing on purpose. Naming such a target on a row is honest; linking it
 * into a page that does not exist is not.
 */
export function recordRoute(
  entityType: string | null | undefined,
  entityID: string | null | undefined,
): Route | undefined {
  if (!entityID || !entityType || !isEntityKind(entityType)) {
    return undefined;
  }
  return ENTITY[entityType].route(entityID);
}

// The reverse of ENTITY[kind].route: which record kind a screen's `id` segment
// names, so the breadcrumb can show "Anna Weber" instead of an opaque id. It is
// DERIVED from the routes rather than restated, because a hand-written copy goes
// stale silently — a new kind whose screen is missing here degrades the crumb to
// a raw uuid with nothing to catch it. A screen absent from the routes has no
// record segment worth resolving and is absent here too.
export const SCREEN_ENTITY: Readonly<Record<string, EntityKind>> =
  Object.fromEntries(
    ENTITY_KINDS.map((kind) => [ENTITY[kind].route("").screen, kind]),
  );
