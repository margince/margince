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
