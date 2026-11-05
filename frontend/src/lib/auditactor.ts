// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { MessageKey } from "../i18n/en";

type AuditLogEntry = components["schemas"]["AuditLogEntry"];
export type ActorFields = Pick<
  AuditLogEntry,
  | "actor_type"
  | "actor_id"
  | "actor_name"
  | "on_behalf_of"
  | "on_behalf_of_name"
  | "passport_id"
> & {
  // The tool's registered name, when the write came through an OAuth grant.
  // Optional because the compliance log's own entry does not carry it yet.
  // Record history does, and both surfaces render through this one component.
  agent_client?: string | null;
};

const ACTOR_TYPES: Record<AuditLogEntry["actor_type"], true> = {
  human: true,
  agent: true,
  system: true,
  connector: true,
  buyer: true,
};

// A server newer than this client may send an actor kind it cannot attribute.
export function isAuditActorType(
  actorType: string,
): actorType is AuditLogEntry["actor_type"] {
  return Object.hasOwn(ACTOR_TYPES, actorType);
}

// The phrase that says a machine did the typing, qualifying the contact who
// authorised it. Only the two delegated actor kinds have one: a human needs no
// qualifier and `system` is not acting for anybody.
const MACHINE_QUALIFIER: Partial<
  Record<AuditLogEntry["actor_type"], "audit.viaAgent" | "audit.viaConnector">
> = {
  agent: "audit.viaAgent",
  connector: "audit.viaConnector",
};

// HUMAN_ACTOR_PREFIX is how the wire spells a human actor: audit_log.actor_id
// carries the typed principal id, not a bare uuid. Comparing the raw column to
// a bare user id once stopped "You" from rendering, because the strings could
// never be equal.
export const HUMAN_ACTOR_PREFIX = "human:";

// actorAttribution decides who a row is attributed to. A buyer is named; a
// member's id is never the label.
//
// The contact comes first and the machine second (PD-002).
// An agent's own identifier is never the label. Attribution exists so somebody
// can be asked about a change, and a passport uuid cannot be asked anything.
// `identifier` is the machine's id, carried only when nothing else is left.
export function actorAttribution(
  entry: ActorFields,
  meUserId: string | undefined,
): {
  labelKey?: MessageKey;
  name?: string;
  qualifierKey?: MessageKey;
  // The tool's own name, when the row records which one. Mutually exclusive
  // with qualifierKey: a named client replaces the generic phrase rather than
  // appending to it.
  qualifierName?: string;
  identifier?: string;
} {
  if (entry.actor_type === "human") {
    if (meUserId && entry.actor_id === HUMAN_ACTOR_PREFIX + meUserId) {
      return { labelKey: "audit.you" };
    }
    // No app_user resolved: the account is gone, or the row carries an id no
    // directory entry matches. Neither is a name, and neither is a uuid worth
    // putting in front of a reader.
    return entry.actor_name
      ? { name: entry.actor_name }
      : { labelKey: "audit.unknownMember" };
  }
  if (entry.actor_type === "system") {
    return entry.passport_id
      ? {
          labelKey: "audit.noHumanAuthority",
          identifier: machineIdentifier(entry),
        }
      : { labelKey: "audit.system" };
  }
  // A Deal Room participant, named the way a member is. The read path resolves
  // actor_name from deal_room_participant for a buyer, so "who confirmed v5?"
  // answers with the contact rather than with their kind.
  //
  // The kind stands in when the name is absent. A revoked participant, an
  // archived room or a subject erasure can leave the audit row without one.
  // An invented name would be worse than the label.
  if (entry.actor_type === "buyer") {
    return entry.actor_name
      ? { name: entry.actor_name, qualifierKey: "audit.viaDealRoom" }
      : { labelKey: "audit.unknownBuyer", qualifierKey: "audit.viaDealRoom" };
  }

  const qualifierKey = MACHINE_QUALIFIER[entry.actor_type];
  // The tool's own name beats the generic phrase. "via Claude" tells a reader
  // which connection wrote the row; "via an agent" tells them what they could
  // already see from the icon.
  const qualifier = entry.agent_client
    ? { qualifierName: entry.agent_client }
    : { qualifierKey };
  // A machine acting under a human's authority reads as that human.
  const authority = humanAuthority(entry, meUserId);
  if (authority) {
    return { ...authority, ...qualifier };
  }
  if (entry.passport_id) {
    // A grant was presented and yet no human resolved behind it. That is a
    // gap: the granting human is recorded when the grant is made, so the row
    // failed to carry what existed. It says so and does not fall back to
    // "System". System is reserved for a change with nobody behind it, and
    // absorbing the gap would hide it on the surface that exists to expose it.
    return {
      labelKey: "audit.noHumanAuthority",
      identifier: machineIdentifier(entry),
      ...qualifier,
    };
  }
  // No grant was presented, so there is no human to name and no gap to report.
  // Examples are a background pass nobody's context ran, or a connector with no
  // connect flow. The deciding question is whether a grant was presented, not which
  // machine word actor_type carries. The id is the readable, workspace-chosen
  // name of the thing that acted.
  return { identifier: machineIdentifier(entry), ...qualifier };
}

// A recorded human no directory entry names is an unknown member, not a gap.
function humanAuthority(
  entry: ActorFields,
  meUserId: string | undefined,
): { labelKey: MessageKey } | { name: string } | undefined {
  if (entry.on_behalf_of && meUserId && entry.on_behalf_of === meUserId) {
    return { labelKey: "audit.you" };
  }
  if (entry.on_behalf_of_name) {
    return { name: entry.on_behalf_of_name };
  }
  return entry.on_behalf_of ? { labelKey: "audit.unknownMember" } : undefined;
}

// machineIdentifier is the machine's own id, with a typed fallback for the one
// state the contract permits and no writer should produce.
// actor_id is declared as a bare `string` with no minimum length.
// An empty one would render an icon with no label, a row attributed to nothing.
function machineIdentifier(entry: ActorFields): string {
  // A registered client name, when there is one. `agent:01a0216f-…` is true
  // but unreadable. A reader asking what changed their record cannot look up
  // a uuid.
  if (entry.agent_client) {
    return entry.agent_client;
  }
  return entry.actor_id === "" ? entry.actor_type : entry.actor_id;
}
