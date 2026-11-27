import { humanizeToken } from "../../format/humanizetoken";

export { humanizeToken } from "../../format/humanizetoken";

import {
  Bot,
  CircleUser,
  Cog,
  type LucideIcon,
  Plug,
  UserRoundCheck,
} from "lucide-react";
import type { components } from "../../api/schema";
import { Badge } from "../../design-system/atoms";
import { formatDateTime } from "../../format/format";
import { type Translator, useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import {
  type ActorFields,
  actorAttribution,
  qualifierPhrase,
} from "../../lib/auditactor";
import { SEARCH_HIT_KIND_KEY } from "../../lib/searchkinds";
import { useViewerZone } from "../../lib/viewerzone";
import { AUDIT_KIND_LABEL } from "./audit.kinds";
import "./audit.css";

// Reusable audit attribution — turns a raw AuditLogEntry into a line a contact
// can read, and specifically into a line that names A CONTACT: the wire carries
// opaque ids (`human:<uuid>`, `agent:<passport>`) plus the display names the
// read path resolves for them. Shared by every audit surface (the settings
// audit log, the custom-fields change rail) so attribution reads the same
// everywhere.

type AuditLogEntry = components["schemas"]["AuditLogEntry"];
// One label per contract action, for the dial and the rows. An action the
// contract adds fails to compile here until it has one.
const AUDIT_ACTION_LABEL = {
  create: "audit.action.create",
  update: "audit.action.update",
  archive: "audit.action.archive",
  merge: "audit.action.merge",
  promote: "audit.action.promote",
  demote: "audit.action.demote",
  disqualify: "audit.action.disqualify",
  restore: "audit.action.restore",
  export: "audit.action.export",
  erase: "audit.action.erase",
  anonymize: "audit.action.anonymize",
  assign: "audit.action.assign",
  advance_stage: "audit.action.advanceStage",
  advance_phase: "audit.action.advancePhase",
  send_email: "audit.action.sendEmail",
  consent_grant: "audit.action.consentGrant",
  consent_withdraw: "audit.action.consentWithdraw",
  approve: "audit.action.approve",
  reject: "audit.action.reject",
  record_share: "audit.action.recordShare",
  record_unshare: "audit.action.recordUnshare",
  activity_relink: "audit.action.activityRelink",
  import: "audit.action.import",
  import_undo: "audit.action.importUndo",
  reset_data: "audit.action.resetData",
  password_link_issued: "audit.action.passwordLinkIssued",
  mfa_removed: "audit.action.mfaRemoved",
  connect: "audit.action.connect",
  disconnect: "audit.action.disconnect",
  schedule: "audit.action.schedule",
  reschedule: "audit.action.reschedule",
  cancel: "audit.action.cancel",
  release: "audit.action.release",
  hold: "audit.action.hold",
  expire: "audit.action.expire",
  resolve: "audit.action.resolve",
  restrict: "audit.action.restrict",
  pin: "audit.action.pin",
  accrue: "audit.action.accrue",
  pay: "audit.action.pay",
  publish: "audit.action.publish",
  pause: "audit.action.pause",
  resume: "audit.action.resume",
  close: "audit.action.close",
  invite: "audit.action.invite",
  revoke: "audit.action.revoke",
  delete: "audit.action.delete",
  place_legal_hold: "audit.action.placeLegalHold",
  lift_legal_hold: "audit.action.liftLegalHold",
  request_legal_review: "audit.action.requestLegalReview",
  withdraw_legal_review: "audit.action.withdrawLegalReview",
} as const satisfies Record<AuditLogEntry["action"], MessageKey>;

const ACTION_LABELS = new Map<string, MessageKey>(
  Object.entries(AUDIT_ACTION_LABEL),
);

export const AUDIT_ACTION_VALUES: readonly string[] = [...ACTION_LABELS.keys()];

// A server newer than this screen may send an action it has no label for yet.
export function actionLabel(action: string, t: Translator): string {
  const key = ACTION_LABELS.get(action);
  return key ? t(key) : humanizeToken(action);
}

const KIND_LABELS = new Map<string, MessageKey>([
  ...Object.entries(SEARCH_HIT_KIND_KEY),
  ...Object.entries(AUDIT_KIND_LABEL),
  ["custom_field", "audit.kind.customField"],
]);

export const AUDIT_ENTITY_TYPE_VALUES: readonly string[] = [
  ...KIND_LABELS.keys(),
];

// A type newer than this client still reads as words rather than as a key.
export function entityLabel(entityType: string, t: Translator): string {
  const key = KIND_LABELS.get(entityType);
  return key ? t(key) : humanizeToken(entityType);
}

export const ACTOR_ICON: Record<AuditLogEntry["actor_type"], LucideIcon> = {
  human: CircleUser,
  agent: Bot,
  system: Cog,
  connector: Plug,
  // A contact, not a machine: a buyer decided this. The icon differs from a
  // member's so a reader can tell at a glance that the actor sits outside the
  // company and will not be found in the member directory.
  buyer: UserRoundCheck,
};

// ActorTag renders WHO acted, naming the CONTACT and saying a machine did the
// typing second (PD-002). A rep working through a passport reads as the rep,
// qualified "via an agent" — never as the agent with the rep in a footnote.
// Shared by every audit surface so attribution reads the same everywhere.
export function ActorTag({
  entry,
  meUserId,
}: Readonly<{ entry: ActorFields; meUserId?: string }>) {
  const t = useT();
  const Icon = ACTOR_ICON[entry.actor_type];
  const who = actorAttribution(entry, meUserId);
  const { labelKey, name, identifier } = who;
  const qualifier = qualifierPhrase(who, t);

  return (
    <span className="audit-actor">
      <Icon aria-hidden />
      {/* The contact is its own element, not a bare text node: it is the
          primary half of the attribution, and a reader (or a test) should be
          able to address who acted separately from what they acted through. */}
      <span className="audit-actor-name">
        {name ?? (labelKey ? t(labelKey) : null)}
      </span>
      {identifier && <span>{identifier}</span>}
      {qualifier && <span>{qualifier}</span>}
    </span>
  );
}

// Kinds whose change `label` is the record's own name. On other kinds a field
// called `label` may hold any value, so the line shows none.
const NAMED_BY_CHANGE: ReadonlySet<string> = new Set(["custom_field"]);

// The live name when the server resolves one, else the name the change wrote.
export function entryName(entry: AuditLogEntry): string | undefined {
  const live = entry.entity_label?.trim();
  if (live) return live;
  if (!NAMED_BY_CHANGE.has(entry.entity_type)) return undefined;
  return [entry.after?.label, entry.before?.label].find(
    (label): label is string =>
      typeof label === "string" && label.trim() !== "",
  );
}

// AuditEntryLine is the one-line rendering of an audit entry: who, what, on
// which kind of thing, and when. The entity's uuid is deliberately dropped —
// it is opaque to a reader and carries no meaning without a name lookup.
export function AuditEntryLine({
  entry,
  meUserId,
  changes = [],
  target,
}: Readonly<{
  entry: AuditLogEntry;
  meUserId?: string;
  /** What the change did, from a surface that reads more than the verb. */
  changes?: readonly string[];
  /** The record by name, from a surface that holds it, for kind and name. */
  target?: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const readerZone = useViewerZone();
  const name = entryName(entry);
  const verbs = changes.length > 0 ? changes : [actionLabel(entry.action, t)];
  return (
    <div className="audit-line t-sub">
      <ActorTag entry={entry} meUserId={meUserId} />
      {verbs.map((verb) => (
        <Badge key={verb} tone="accent">
          {verb}
        </Badge>
      ))}
      {target ? (
        <span className="audit-target">{target}</span>
      ) : (
        <>
          <span>{entityLabel(entry.entity_type, t)}</span>
          {name && <span className="audit-target">{name}</span>}
        </>
      )}
      {/* An audit entry is a fact in the company's book, like the change
          history beside it, so it reads on the company's clock. On the
          viewer's clock an entry at 18:00Z is 21 August to a reader in Berlin
          and 22 August to one in Ho Chi Minh City: two investigators quoting
          the same line quote different days, which is the failure a shared
          record clock exists to prevent. `dateTime` carries the unambiguous
          instant for anyone who has to convert. */}
      <time className="audit-when" dateTime={entry.occurred_at}>
        {formatDateTime(entry.occurred_at, locale, readerZone)}
      </time>
    </div>
  );
}
