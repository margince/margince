import type { MessengerContext, MessengerScope } from "./messenger.queries";

export const messengerScope: MessengerScope = {
  provider: "slack",
  record_type: "deal",
  id: "018f3a1b-0000-7000-8000-00000000c001",
};
export const messengerContext: MessengerContext = {
  provider: "slack",
  workspace: "T_DEMO",
  grant_version: 1,
  audience_key: "synthetic-roster",
  sender: { id: "U_SELF", name: "Alex Morgan", active: true, external: false },
  conversation: {
    id: "C_PILOT",
    name: "Pilot rollout",
    kind: "channel",
    audience_known: true,
    external: false,
    members: [
      { id: "U_SELF", name: "Alex Morgan", active: true, external: false },
      { id: "U_JORDAN", name: "Jordan Lee", active: true, external: false },
    ],
  },
};
