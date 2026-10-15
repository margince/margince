import type { components } from "../api/schema";

type LinkedInReach = components["schemas"]["LinkedInReachResponse"];

/** A `/me/linkedin-reach` body in the contract's full shape: every field, typed. */
export function linkedInReachFixture(
  accounts: LinkedInReach["accounts"] = [],
  unresolved = 0,
): LinkedInReach {
  return {
    accounts,
    accounts_total: accounts.length,
    unresolved_connections: unresolved,
  };
}
