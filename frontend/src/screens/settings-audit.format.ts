// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import type { BADGE_TONES } from "../design-system/atoms";
import { stable } from "../format/collate";
import type { MessageKey } from "../i18n/en";

export type AuditLogEntry = components["schemas"]["AuditLogEntry"];

type RecordImage = AuditLogEntry["before"];

export type AuditLogFilters = Readonly<{
  actor: string;
  entityType: string;
  entityId: string;
  action: string;
  from: string;
  to: string;
}>;

export const UNFILTERED_AUDIT_LOG: AuditLogFilters = {
  actor: "",
  entityType: "",
  entityId: "",
  action: "",
  from: "",
  to: "",
};

export const AUDIT_LOG_FILTER_FIELDS: readonly Readonly<{
  key: keyof AuditLogFilters;
  labelKey: MessageKey;
  date?: boolean;
}>[] = [
  { key: "actor", labelKey: "settings.auditActor" },
  { key: "entityType", labelKey: "settings.auditEntity" },
  { key: "entityId", labelKey: "settings.auditEntityId" },
  { key: "action", labelKey: "settings.auditAction" },
  { key: "from", labelKey: "settings.auditFrom", date: true },
  { key: "to", labelKey: "settings.auditTo", date: true },
];

export const AUDIT_LOG_PAGE_SIZE = 20;

export function isNarrowed(filters: AuditLogFilters): boolean {
  return Object.values(filters).some((value) => value.trim() !== "");
}

// A date input's day, read as a UTC instant: the `to` end takes the whole day.
function fromDateParam(date: string): string {
  return new Date(`${date}T00:00:00.000Z`).toISOString();
}
function toDateParam(date: string): string {
  return new Date(`${date}T23:59:59.999Z`).toISOString();
}

export function auditLogQueryParams(
  filters: AuditLogFilters,
  pageParam: string | null,
) {
  const { actor, entityType, entityId, action, from, to } = filters;
  return {
    limit: AUDIT_LOG_PAGE_SIZE,
    ...(pageParam ? { cursor: pageParam } : {}),
    ...(actor.trim() ? { actor: actor.trim() } : {}),
    ...(entityType.trim() ? { entity_type: entityType.trim() } : {}),
    ...(entityId.trim() ? { entity_id: entityId.trim() } : {}),
    ...(action.trim() ? { action: action.trim() } : {}),
    ...(from ? { from: fromDateParam(from) } : {}),
    ...(to ? { to: toDateParam(to) } : {}),
  };
}

export const ACTION_TONE: Readonly<
  Record<string, (typeof BADGE_TONES)[number]>
> = {
  create: "success",
  delete: "danger",
};

// The shape of the images decides how a row reads, not the verb. An archive or
// a merge that carries both images is an update to the reader.
export type ChangeShape = "created" | "changed" | "removed";

export function changeShape(entry: AuditLogEntry): ChangeShape {
  if (!entry.before) {
    return "created";
  }
  return entry.after ? "changed" : "removed";
}

// Sorted by code point: these are the record's own column names, and one entry
// must list them in one order whatever the reader's language.
export function diffKeys(before: RecordImage, after: RecordImage): string[] {
  return [
    ...new Set([...Object.keys(before ?? {}), ...Object.keys(after ?? {})]),
  ].sort(stable);
}

export type FieldValue = Readonly<{ text: string; structured: boolean }>;

// An absent key and an explicit null both read as no value, so a side the
// image lacks is never given one.
export function fieldValue(image: RecordImage, key: string): FieldValue | null {
  const value = image?.[key];
  if (value === null || value === undefined) {
    return null;
  }
  return typeof value === "object"
    ? { text: JSON.stringify(value), structured: true }
    : { text: String(value), structured: false };
}

const POLICY_RULE = /^role\[([^\]]*)\] ([^.\s]+)\.(\S+) row_scope=(\S*)$/;

export type AuthorizationRule =
  | Readonly<{
      kind: "policy";
      roles: readonly string[];
      object: string;
      action: string;
      scope: string;
    }>
  | Readonly<{ kind: "system" | "dealRoom" }>
  | Readonly<{ kind: "raw"; text: string }>;

// The server writes `role[a,b] object.action row_scope=s`, or a bare word for
// a write no role policy admitted; anything else is shown as written.
export function parseAuthorizationRule(rule: string): AuthorizationRule {
  if (rule === "system") {
    return { kind: "system" };
  }
  if (rule === "deal_room_session") {
    return { kind: "dealRoom" };
  }
  const match = POLICY_RULE.exec(rule);
  if (!match) {
    return { kind: "raw", text: rule };
  }
  const [, roles, object, action, scope] = match;
  return {
    kind: "policy",
    roles: roles.split(",").filter(Boolean),
    object,
    action,
    scope,
  };
}

export const ROW_SCOPE_WORDS: Readonly<Record<string, MessageKey>> = {
  own: "settings.auditScopeOwn",
  team: "settings.auditScopeTeam",
  all: "settings.auditScopeAll",
};

// The tail, not the head: a uuidv7 leads with its timestamp. Every recent id
// shares its first characters, and only the random end tells two apart.
export function idTail(id: string): string {
  return id.slice(-8);
}
