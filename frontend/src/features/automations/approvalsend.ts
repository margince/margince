import type { components } from "../../api/schema";
import type { Approval } from "../../lib/approvals.queries";
import { isRecord } from "../../lib/problembody";
import { RELINK_KINDS, type RelinkKind } from "../mail/compose";

// The message a staged proposal would send, read back out of the proposal.
//
// An approver releasing somebody else's mail deserves the same answer the
// composer gives its author: whether the engine would refuse it, and whose
// decision that is. The proposal carries what the engine needs — addressees,
// the anchor or the records, the claim — but under the kind's own payload
// shape, and `proposed_change` reaches the browser as an untyped document. So
// this reads each shape as an ADMISSION: a field is used only when it has the
// type the staging code writes, and a payload that does not describe a mail
// yields no question rather than a guessed one.
//
// A kind absent here is not asked about, and each absence is a fact about the
// message rather than an omission: a channel reply names no addressee anybody
// can preview, and a held scheduled send is answered on the queue that holds
// it, beside the controls that move it.

type CommunicationContext = components["schemas"]["CommunicationContext"];
type Link = components["schemas"]["ActivityLinkInput"];
type Evidence = components["schemas"]["CommunicationEvidence"];

/** What the preview hook is asked, for one staged send. */
export type StagedSend = Readonly<{
  recipients: readonly string[];
  anchorActivityId?: string;
  links?: readonly Link[];
  context?: CommunicationContext;
  marketingPurpose?: string;
  consentPurpose?: string;
  evidence?: Evidence;
}>;

function stringOf(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() !== "" ? value : undefined;
}

/**
 * An address list, whichever way the kind spells it: the automation's held
 * draft names ONE addressee as a string, the agent tools name a list.
 */
function addressesOf(value: unknown): string[] {
  if (typeof value === "string") {
    return value.trim() === "" ? [] : [value];
  }
  if (!Array.isArray(value)) {
    return [];
  }
  return value.filter(
    (entry): entry is string =>
      typeof entry === "string" && entry.trim() !== "",
  );
}

function isRelinkKind(value: unknown): value is RelinkKind {
  return (
    typeof value === "string" &&
    (RELINK_KINDS as readonly string[]).includes(value)
  );
}

/**
 * The records an account-started send names. Every entry must be a whole link:
 * a list with one malformed entry is not a shorter list, it is a payload this
 * build does not understand, and the question is not asked.
 */
function linksOf(value: unknown): Link[] | undefined {
  if (!Array.isArray(value)) {
    return undefined;
  }
  const links: Link[] = [];
  for (const entry of value) {
    if (typeof entry !== "object" || entry === null) {
      return undefined;
    }
    const type: unknown = Reflect.get(entry, "entity_type");
    const id = stringOf(Reflect.get(entry, "entity_id"));
    if (!isRelinkKind(type) || id === undefined) {
      return undefined;
    }
    links.push({ entity_type: type, entity_id: id });
  }
  return links;
}

/**
 * The claim, only when it is one this build can pass on.
 *
 * Spelled as the contract's enum rather than accepted as any string, because
 * the preview door refuses a value outside it and a refused preview reads as
 * "could not check" on a message that merely carried an unknown word. Dropped,
 * the engine derives the category from the record, which is what an absent
 * claim means on every other path.
 */
function contextOf(value: unknown): CommunicationContext | undefined {
  switch (value) {
    case "reply_to_inbound":
    case "requested_followup":
    case "precontract_quote":
    case "active_deal_followup":
    case "customer_service":
    case "account_notice":
    case "contract_notice":
    case "invoice_or_payment":
    case "marketing":
      return value;
    default:
      return undefined;
  }
}

/**
 * What to ask the engine about this proposal, or undefined when it is not a
 * mail anybody can preview.
 */
export function stagedSendOf(approval: Approval): StagedSend | undefined {
  let change = approval.proposed_change ?? {};
  if (Object.hasOwn(change, "operation")) {
    if (!isRecord(change.body)) return undefined;
    change = change.body;
  }
  const question = sendQuestion(change);
  if (question === undefined) return undefined;
  switch (approval.kind) {
    // The automation's reply, waiting for a human to read and release. It names
    // the thread it answers in its own payload.
    case "held_draft": {
      const anchorActivityId = stringOf(change.anchor_activity_id);
      if (anchorActivityId === undefined) {
        return undefined;
      }
      return { ...question, anchorActivityId };
    }
    // An agent's reply. The anchor is the approval's own target: the row the
    // effect hangs off, which for a reply is the activity being answered.
    case "send_email": {
      if (approval.target_entity_type !== "activity") {
        return undefined;
      }
      const anchorActivityId = stringOf(approval.target_entity_id);
      if (anchorActivityId === undefined) {
        return undefined;
      }
      return { ...question, anchorActivityId };
    }
    // An agent's account-started mail: no anchor, so the records it would be
    // filed under are the question's other half.
    case "send_company_email": {
      const links = linksOf(change.links);
      if (links === undefined || links.length === 0) {
        return undefined;
      }
      return { ...question, links };
    }
    default:
      return undefined;
  }
}

function sendQuestion(
  change: Readonly<Record<string, unknown>>,
): StagedSend | undefined {
  const recipients = [
    ...addressesOf(change.to),
    ...addressesOf(change.cc),
    ...addressesOf(change.bcc),
  ];
  if (recipients.length === 0) {
    return undefined;
  }
  const context = contextOf(change.communication_context);
  const marketingPurpose = stringOf(change.marketing_purpose);
  const consentPurpose = stringOf(change.consent_purpose);
  const evidence = evidenceOf(change.evidence);
  if (change.evidence != null && evidence === undefined) return undefined;
  return {
    recipients,
    context,
    marketingPurpose,
    ...(consentPurpose ? { consentPurpose } : {}),
    ...(evidence ? { evidence } : {}),
  };
}

function evidenceOf(value: unknown): Evidence | undefined {
  if (!isRecord(value)) return undefined;
  const fields: Record<keyof Evidence, true> = {
    activity_id: true,
    deal_id: true,
    invoice_id: true,
    contract_id: true,
    consent_event_id: true,
    basis_id: true,
  };
  if (
    Object.entries(value).some(
      ([key, entry]) =>
        !Object.hasOwn(fields, key) ||
        (entry !== null && typeof entry !== "string"),
    )
  )
    return undefined;
  return {
    activity_id: stringOf(value.activity_id),
    deal_id: stringOf(value.deal_id),
    invoice_id: stringOf(value.invoice_id),
    contract_id: stringOf(value.contract_id),
    consent_event_id: stringOf(value.consent_event_id),
    basis_id: stringOf(value.basis_id),
  };
}
