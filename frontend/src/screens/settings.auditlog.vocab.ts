import type { components } from "../api/schema";
import { calendarDay } from "../format/calendarday";
import { endOfDayInZone, startOfDayInZone } from "../format/timezone";
import type { MessageKey } from "../i18n/en";

type AuditAction = components["schemas"]["AuditLogEntry"]["action"];

// Every action the contract can carry, as the past-tense word a reader sees.
// A Record over the wire's own union, so a new action the contract gains fails
// the type check here instead of rendering as a raw token.
export const AUDIT_ACTION_LABEL: Record<AuditAction, MessageKey> = {
  create: "settings.auditLog.action.create",
  update: "settings.auditLog.action.update",
  archive: "settings.auditLog.action.archive",
  merge: "settings.auditLog.action.merge",
  promote: "settings.auditLog.action.promote",
  demote: "settings.auditLog.action.demote",
  disqualify: "settings.auditLog.action.disqualify",
  restore: "settings.auditLog.action.restore",
  export: "settings.auditLog.action.export",
  erase: "settings.auditLog.action.erase",
  anonymize: "settings.auditLog.action.anonymize",
  assign: "settings.auditLog.action.assign",
  advance_stage: "settings.auditLog.action.advance_stage",
  advance_phase: "settings.auditLog.action.advance_phase",
  send_email: "settings.auditLog.action.send_email",
  consent_grant: "settings.auditLog.action.consent_grant",
  consent_withdraw: "settings.auditLog.action.consent_withdraw",
  approve: "settings.auditLog.action.approve",
  reject: "settings.auditLog.action.reject",
  record_share: "settings.auditLog.action.record_share",
  record_unshare: "settings.auditLog.action.record_unshare",
  activity_relink: "settings.auditLog.action.activity_relink",
  import: "settings.auditLog.action.import",
  import_undo: "settings.auditLog.action.import_undo",
  reset_data: "settings.auditLog.action.reset_data",
  password_link_issued: "settings.auditLog.action.password_link_issued",
  connect: "settings.auditLog.action.connect",
  disconnect: "settings.auditLog.action.disconnect",
  schedule: "settings.auditLog.action.schedule",
  reschedule: "settings.auditLog.action.reschedule",
  cancel: "settings.auditLog.action.cancel",
  release: "settings.auditLog.action.release",
  hold: "settings.auditLog.action.hold",
  expire: "settings.auditLog.action.expire",
  resolve: "settings.auditLog.action.resolve",
  restrict: "settings.auditLog.action.restrict",
  pin: "settings.auditLog.action.pin",
  accrue: "settings.auditLog.action.accrue",
  pay: "settings.auditLog.action.pay",
  publish: "settings.auditLog.action.publish",
  pause: "settings.auditLog.action.pause",
  resume: "settings.auditLog.action.resume",
  close: "settings.auditLog.action.close",
  invite: "settings.auditLog.action.invite",
  revoke: "settings.auditLog.action.revoke",
  delete: "settings.auditLog.action.delete",
  place_legal_hold: "settings.auditLog.action.place_legal_hold",
  lift_legal_hold: "settings.auditLog.action.lift_legal_hold",
};

// The order the Action dial offers them in: the contract's own order, which
// puts the everyday verbs first.
export const AUDIT_ACTIONS = Object.keys(AUDIT_ACTION_LABEL) as AuditAction[];

// Data leaving or being destroyed is what an auditor scans for, so those
// actions stand out; every routine write stays neutral.
const DESTRUCTIVE: ReadonlySet<AuditAction> = new Set([
  "erase",
  "delete",
  "anonymize",
  "reset_data",
]);

export function auditActionTone(
  action: AuditAction,
): "default" | "warning" | "danger" {
  if (DESTRUCTIVE.has(action)) {
    return "danger";
  }
  return action === "export" ? "warning" : "default";
}

// Readable names for the record types the trail carries most. entity_type is a
// free string on the wire, so a type missing here still reads through
// `sentenceCaseToken` rather than as a raw table name.
export const AUDIT_TYPE_LABEL: Readonly<Record<string, MessageKey>> = {
  contact: "settings.auditLog.type.contact",
  company: "settings.auditLog.type.company",
  deal: "settings.auditLog.type.deal",
  lead: "settings.auditLog.type.lead",
  project: "settings.auditLog.type.project",
  activity: "settings.auditLog.type.activity",
  user: "settings.auditLog.type.user",
  offer: "settings.auditLog.type.offer",
  offer_template: "settings.auditLog.type.offer_template",
  product: "settings.auditLog.type.product",
  pipeline: "settings.auditLog.type.pipeline",
  stage: "settings.auditLog.type.stage",
  tag: "settings.auditLog.type.tag",
  automation: "settings.auditLog.type.automation",
  attachment: "settings.auditLog.type.attachment",
  relationship: "settings.auditLog.type.relationship",
  weekly_review: "settings.auditLog.type.weekly_review",
  forecast_snapshot: "settings.auditLog.type.forecast_snapshot",
  forecast_capture_status: "settings.auditLog.type.forecast_capture_status",
  onboarding_wizard_state: "settings.auditLog.type.onboarding_wizard_state",
  privacy_notice_case: "settings.auditLog.type.privacy_notice_case",
  data_subject_request: "settings.auditLog.type.data_subject_request",
  provider_connection: "settings.auditLog.type.provider_connection",
  saved_view: "settings.auditLog.type.saved_view",
};

// "forecast_capture_status" as "Forecast capture status": the fallback for a
// type no label covers, which is still a word and never a table name.
export function sentenceCaseToken(token: string): string {
  const spaced = token.replace(/_/g, " ");
  return spaced.charAt(0).toUpperCase() + spaced.slice(1);
}

// Enough of a uuid to tell two rows apart at a glance; the whole id stays in
// the change detail.
export function shortId(id: string): string {
  return id.slice(0, 8);
}

export type AuditWhen = "any" | "today" | "last7" | "last30" | "custom";

// The When dial as the server's from/to pair, cut on the company clock: the
// same clock every time in the table reads on, so "today" means the day the
// rows say it is rather than the UTC one.
export function auditWhenRange(
  when: AuditWhen,
  custom: Readonly<{ from: string; to: string }>,
  now: Date,
  zone: string,
): { from?: string; to?: string } {
  const today = calendarDay(now, zone);
  switch (when) {
    case "today":
      return { from: startOfDayInZone(today, zone) };
    case "last7":
      return { from: startOfDayInZone(today, zone, -6) };
    case "last30":
      return { from: startOfDayInZone(today, zone, -29) };
    case "custom":
      return {
        ...(custom.from ? { from: startOfDayInZone(custom.from, zone) } : {}),
        ...(custom.to ? { to: endOfDayInZone(custom.to, zone) } : {}),
      };
    default:
      return {};
  }
}
