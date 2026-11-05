import type { components } from "../api/schema";
import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { actorAttribution, isAuditActorType } from "./auditactor";
import { HISTORY_VERB } from "./historyverbs";

// One history entry as a sentence in the reader's language. A link change, or a
// verb or actor kind newer than this client, keeps the server's English summary.

type AuditHistoryEntry = components["schemas"]["AuditHistoryEntry"];

const VERB_KEYS = new Map<string, MessageKey>(Object.entries(HISTORY_VERB));

// A server newer than this client may send a verb or actor kind unknown here.
// So both are read as plain strings.
export type HistorySubjectFacts = Pick<
  AuditHistoryEntry,
  | "actor_id"
  | "actor_name"
  | "on_behalf_of"
  | "on_behalf_of_name"
  | "agent_client"
  | "passport_id"
> & { actor_type: string };

export type HistoryLineFacts = HistorySubjectFacts &
  Pick<AuditHistoryEntry, "edge" | "summary"> & { action: string };

type Attribution = ReturnType<typeof actorAttribution>;

// The subject for an attribution that names a contact: the name, then the
// tool or the Deal Room it acted through.
function namedSubject(who: Attribution, name: string, t: Translator): string {
  if (who.qualifierName) {
    return t("history.subject.viaClient", { name, client: who.qualifierName });
  }
  switch (who.qualifierKey) {
    case "audit.viaAgent":
      return t("history.subject.viaAgent", { name });
    case "audit.viaConnector":
      return t("history.subject.viaConnector", { name });
    case "audit.viaDealRoom":
      return t("history.subject.buyerNamed", { name });
    default:
      return name;
  }
}

/**
 * Who made a change, as the opening of a history sentence, from the same
 * attribution the audit log shows. Null for an actor kind this client does not
 * know, which leaves the line to `summary`.
 */
export function historySubject(
  facts: HistorySubjectFacts,
  t: Translator,
): string | null {
  const actorType = facts.actor_type;
  if (!isAuditActorType(actorType)) return null;
  const who = actorAttribution({ ...facts, actor_type: actorType }, undefined);
  switch (who.labelKey) {
    case "audit.noHumanAuthority":
      return t("history.subject.unrecordedAuthority");
    case "audit.system":
      return t("history.subject.system");
    case "audit.unknownBuyer":
      return t("history.subject.buyer");
    case "audit.unknownMember":
      return namedSubject(who, t("audit.unknownMember"), t);
    default:
      break;
  }
  if (who.name) return namedSubject(who, who.name, t);
  if (actorType === "agent") return t("history.subject.agent");
  if (actorType === "connector") return t("history.subject.connector");
  return null;
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
