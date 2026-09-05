import type { components } from "../api/schema";
import type { RbacAction, RbacObject } from "./capability";

// The /me body a test serves, built from the grants under test rather than
// from a role name.
//
// Tests used to stub `roles: ["admin"]` and rely on the screen inferring
// capability from it. That made every assertion a statement about the seeded
// matrix — so a screen wired to the WRONG object still passed, because admin
// holds everything and one wrong grant looks exactly like the right one.
//
// Naming the grants makes the binding testable: a fixture that allows
// `automation:update` and nothing else proves the screen asks for exactly
// that. Divergent fixtures (one object granted, its neighbour not; one action
// granted, its sibling not) are the point, not an edge case.

type MeResponse = components["schemas"]["MeResponse"];
type RbacObjectGrant = components["schemas"]["RbacObjectGrant"];
type RowScope = NonNullable<
  components["schemas"]["Authorization"]
>["row_scope"];

const NO_GRANT: RbacObjectGrant = {
  create: false,
  read: false,
  update: false,
  delete: false,
};

export type GrantSpec = Partial<Record<RbacObject, readonly RbacAction[]>>;

/**
 * A /me response granting exactly `allow` and nothing else.
 *
 * Objects absent from `allow` are omitted entirely rather than written as an
 * all-false grant — that is what the server does for an object a role was
 * never granted, so a client that mistakes an absent key for an unrestricted
 * one fails here rather than in production.
 */
export function meFixture({
  roles = ["admin"],
  seat = "full",
  allow = {},
  rowScope = "own",
  settingsAvailability = { company_context: true },
}: {
  roles?: string[];
  seat?: "full" | "read";
  allow?: GrantSpec;
  /**
   * How far the grants reach. Defaults to `own`, the narrowest, for the same
   * reason the server does: a fixture that does not state a scope describes a
   * principal whose scope was never resolved, and reading that as `all` would
   * make every test agree with a widening nobody wrote.
   */
  rowScope?: RowScope;
  /**
   * Which settings surfaces exist in this installation. Defaults to the company
   * page being available, because that is the compiled default the deployment
   * config resolves to — a fixture omitting it should describe an ordinary
   * installation, not one with the rollout switched off.
   *
   * Pass `null` for the third state, which is neither of those: a server older
   * than the field, or a snapshot cached before it shipped, answers /me with no
   * `settings_availability` at all. That is not the same record as one saying
   * the surface is off, and a reader has to fail closed on both — so the
   * absence needs its own spelling rather than riding the default.
   */
  settingsAvailability?: MeResponse["settings_availability"] | null;
} = {}): MeResponse {
  const objects: Record<string, RbacObjectGrant> = {};
  for (const [object, actions] of Object.entries(allow)) {
    const grant: RbacObjectGrant = { ...NO_GRANT };
    for (const action of actions ?? []) {
      grant[action] = true;
    }
    objects[object] = grant;
  }
  const me: MeResponse = {
    user: {
      id: "00000000-0000-4000-8000-000000000001",
      email: "test@example.test",
      display_name: "Test User",
      timezone: "Europe/Berlin",
      status: "active",
      is_agent: false,
    },
    roles,
    teams: [],
    workspace_name: "Test Workspace",
    non_production: false,
    data_reset_available: false,
    admin_password_link: false,
    authorization: { seat_type: seat, objects, row_scope: rowScope },
    settings_availability: settingsAvailability ?? undefined,
  };
  return me;
}
