// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import type { BADGE_TONES } from "../../design-system/atoms";
import { stable } from "../../format/collate";
import type { MessageKey } from "../../i18n/en";
import {
  AUDIT_ENTITY_ID_PARAM,
  AUDIT_ENTITY_TYPE_PARAM,
} from "../../lib/auditloglink";
import type { UrlParams } from "../../lib/urlstate";

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

// Every dial lives in the address, so a reload or a shared link keeps it. The
// entity pair keeps the spelling a record's history link writes.
const FILTER_PARAMS: Readonly<Record<keyof AuditLogFilters, string>> = {
  actor: "actor",
  entityType: AUDIT_ENTITY_TYPE_PARAM,
  entityId: AUDIT_ENTITY_ID_PARAM,
  action: "action",
  from: "from",
  to: "to",
};

const FILTER_KEYS: readonly (keyof AuditLogFilters)[] = [
  "actor",
  "entityType",
  "entityId",
  "action",
  "from",
  "to",
];

export function auditLogFiltersOf(params: UrlParams): AuditLogFilters {
  const read = (key: keyof AuditLogFilters) =>
    params.get(FILTER_PARAMS[key]) ?? "";
  return {
    actor: read("actor"),
    entityType: read("entityType"),
    entityId: read("entityId"),
    action: read("action"),
    from: read("from"),
    to: read("to"),
  };
}

export function withAuditLogFilters(
  params: UrlParams,
  filters: AuditLogFilters,
): UrlParams {
  const next = new Map(params);
  for (const key of FILTER_KEYS) {
    const value = filters[key].trim();
    if (value) {
      next.set(FILTER_PARAMS[key], value);
    } else {
      next.delete(FILTER_PARAMS[key]);
    }
  }
  return next;
}

const FULL_ID =
  /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;

// The server refuses anything but a whole uuid, so a part of one never leaves
// the field.
export function isFullId(value: string): boolean {
  return FULL_ID.test(value.trim());
}

// Date inputs hold YYYY-MM-DD, which sorts as the days do.
export function isRangeReversed(filters: AuditLogFilters): boolean {
  return filters.from !== "" && filters.to !== "" && filters.from > filters.to;
}

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
  const [, roles = "", object = "", action = "", scope = ""] = match;
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
export function idTail(id: string | null | undefined): string {
  return (id ?? "").slice(-8);
}
