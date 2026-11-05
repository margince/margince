import type { useT } from "../../../i18n";

/** The roles a committee reads in, and the order it reads them. */
export const ROLES = [
  "champion",
  "economic_buyer",
  "influencer",
  "blocker",
  "user",
] as const;

export const ROLE_LABELS = {
  champion: "co.role.champion",
  economic_buyer: "co.role.economic_buyer",
  influencer: "co.role.influencer",
  blocker: "co.role.blocker",
  user: "co.role.user",
} as const;

function isCatalogRole(role: string): role is keyof typeof ROLE_LABELS {
  return Object.hasOwn(ROLE_LABELS, role);
}

/**
 * The reader's word for a recorded buying role. A role is written by
 * convention rather than drawn from an enum. So a value the catalog does not
 * name is shown as written, not as an error. Every record page reads it here,
 * so the account and the contact agree on the word.
 */
export function buyingRoleLabel(
  role: string,
  t: ReturnType<typeof useT>,
): string {
  return isCatalogRole(role) ? t(ROLE_LABELS[role]) : role.replace(/_/g, " ");
}
