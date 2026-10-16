import type { components } from "../api/schema";
import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";

// One history entry as a sentence in the reader's language. A link change, or a
// verb or actor kind newer than this client, keeps the server's English summary.

type AuditHistoryEntry = components["schemas"]["AuditHistoryEntry"];
type AuditAction = components["schemas"]["AuditAction"];

// Each key holds the whole sentence around `{subject}`, because word order is
// the language's: German puts the participle last, Vietnamese the object last.
const HISTORY_VERB = {
  create: "history.verb.create",
  update: "history.verb.update",
  archive: "history.verb.archive",
  merge: "history.verb.merge",
  promote: "history.verb.promote",
  demote: "history.verb.demote",
  disqualify: "history.verb.disqualify",
  restore: "history.verb.restore",
  export: "history.verb.export",
  erase: "history.verb.erase",
  anonymize: "history.verb.anonymize",
  assign: "history.verb.assign",
  advance_stage: "history.verb.advanceStage",
  advance_phase: "history.verb.advancePhase",
  send_email: "history.verb.sendEmail",
  consent_grant: "history.verb.consentGrant",
  consent_withdraw: "history.verb.consentWithdraw",
  approve: "history.verb.approve",
  reject: "history.verb.reject",
  record_share: "history.verb.recordShare",
  record_unshare: "history.verb.recordUnshare",
  activity_relink: "history.verb.activityRelink",
  import: "history.verb.import",
  import_undo: "history.verb.importUndo",
  reset_data: "history.verb.resetData",
  password_link_issued: "history.verb.passwordLinkIssued",
  connect: "history.verb.connect",
  disconnect: "history.verb.disconnect",
  schedule: "history.verb.schedule",
  reschedule: "history.verb.reschedule",
  cancel: "history.verb.cancel",
  release: "history.verb.release",
  hold: "history.verb.hold",
  expire: "history.verb.expire",
  resolve: "history.verb.resolve",
  restrict: "history.verb.restrict",
  pin: "history.verb.pin",
  accrue: "history.verb.accrue",
  pay: "history.verb.pay",
  publish: "history.verb.publish",
  pause: "history.verb.pause",
  resume: "history.verb.resume",
  close: "history.verb.close",
  invite: "history.verb.invite",
  revoke: "history.verb.revoke",
  delete: "history.verb.delete",
  place_legal_hold: "history.verb.placeLegalHold",
  lift_legal_hold: "history.verb.liftLegalHold",
  request_legal_review: "history.verb.requestLegalReview",
  withdraw_legal_review: "history.verb.withdrawLegalReview",
} as const satisfies Record<AuditAction, MessageKey>;

const VERB_KEYS = new Map<string, MessageKey>(Object.entries(HISTORY_VERB));

// A server newer than this client may send a verb or actor kind unknown here.
// So both are read as plain strings.
export type HistorySubjectFacts = Pick<
  AuditHistoryEntry,
  | "actor_id"
  | "actor_name"
  | "on_behalf_of_name"
  | "agent_client"
  | "passport_id"
> & { actor_type: string };

export type HistoryLineFacts = HistorySubjectFacts &
  Pick<AuditHistoryEntry, "edge" | "summary"> & { action: string };

// The contact who authorized a change comes first and the tool second, so a
// line always names somebody who can be asked about it.
function delegatedSubject(
  facts: HistorySubjectFacts,
  t: Translator,
): string | null {
  const name = facts.on_behalf_of_name;
  if (!name) return null;
  if (facts.agent_client) {
    return t("history.subject.viaClient", { name, client: facts.agent_client });
  }
  if (facts.actor_type === "agent") {
    return t("history.subject.viaAgent", { name });
  }
  if (facts.actor_type === "connector") {
    return t("history.subject.viaConnector", { name });
  }
  return null;
}

/**
 * Who made a change, as the opening of a history sentence. Null for an actor
 * kind this client does not know, which leaves the line to `summary`.
 */
export function historySubject(
  facts: HistorySubjectFacts,
  t: Translator,
): string | null {
  const machine =
    facts.actor_type === "agent" || facts.actor_type === "connector";
  const delegated = machine ? delegatedSubject(facts, t) : null;
  if (delegated) return delegated;
  // An unresolved member keeps the id the row recorded, never an invented name.
  if (facts.actor_type === "human") return facts.actor_name || facts.actor_id;
  // A passport with nobody behind it is an authority the row failed to carry,
  // which "System" would hide.
  if (facts.passport_id) return t("history.subject.unrecordedAuthority");
  switch (facts.actor_type) {
    case "system":
      return t("history.subject.system");
    case "agent":
      return t("history.subject.agent");
    case "connector":
      return t("history.subject.connector");
    case "buyer":
      return t("history.subject.buyer");
    default:
      return null;
  }
}

/** The history line for one entry, in the reader's language. */
export function historyLine(entry: HistoryLineFacts, t: Translator): string {
  // An edge line names the other record and the role it was linked as, and
  // the role travels only inside `summary`.
  if (entry.edge) return entry.summary;
  const verb = VERB_KEYS.get(entry.action);
  const subject = historySubject(entry, t);
  if (!verb || subject === null) return entry.summary;
  return t(verb, { subject });
}
