import type { components } from "../api/schema";
import {
  type RefusalKind,
  refusalKindOf,
} from "../format/sendpermissionreason";

// One answer about the message being written. The server gives one in two
// places: the preview asked while the rep writes, and a refused Send. Drawing
// both left a red refusal beside a later check that said the message may go.

type Preview = components["schemas"]["SendAuthorizationPreview"];
type PreviewRecipient = Preview["recipients"][number];

/** One refused recipient, as the server named it. */
export type RefusedRecipient = Readonly<{
  address: string;
  reasonCode: string;
  /** What the engine worked out this message is. */
  category?: string;
  /** The reason for contact the sender picked, which a failed claim never resolves to. */
  requestedCategory?: string;
  /** The server's answer to whether a recorded reason may lift it. */
  canBeOverruled: boolean;
  footerFindings?: readonly components["schemas"]["FooterFinding"][];
  /** The first law cited for the reason, when one may be shown. */
  provision?: components["schemas"]["CitedProvision"];
  /** Every law cited for the reason: the recipient's pack, then the sender's. */
  provisions?: readonly components["schemas"]["CitedProvision"][];
}>;

export type SendStatusKind =
  | "idle"
  | "checking"
  | "ready"
  | "unavailable"
  | RefusalKind;

export type SendStatus = Readonly<{
  kind: SendStatusKind;
  /** The refusal the status explains: the first in the server's order. */
  deciding?: RefusedRecipient;
  /** How many recipients refuse, the deciding one included. */
  refusing: number;
  /** Every refused recipient, in the server's order. */
  refused: readonly RefusedRecipient[];
  /** How many recipients the answer covers; 0 where the server did not say. */
  recipients: number;
}>;

/**
 * The category a refusal's remedy is named for. The sender's own pick comes
 * first: a failed "Active deal" claim resolves to some other category, and its
 * remedy is still the deal, never a missing reason or consent.
 */
export function remedyCategoryOf(
  refusal: Pick<RefusedRecipient, "category" | "requestedCategory">,
): string | undefined {
  return refusal.requestedCategory ?? refusal.category;
}

/** A preview recipient as a refusal. Absent overrulability is not a yes. */
export function refusalOf(recipient: PreviewRecipient): RefusedRecipient {
  return {
    footerFindings: recipient.footer_findings,
    address: recipient.address,
    reasonCode: recipient.reason_code,
    category: recipient.resolved_category,
    requestedCategory: recipient.requested_category,
    canBeOverruled: recipient.can_be_overruled === true,
    provision: recipient.provision,
    provisions: recipient.provisions,
  };
}

function statusOf(
  refusals: readonly RefusedRecipient[],
  recipients: number,
): SendStatus {
  // The server ranks refusals, in the preview and after Send alike, so the
  // first is the one the status explains.
  const deciding = refusals[0];
  return {
    kind: deciding
      ? refusalKindOf(deciding.reasonCode, remedyCategoryOf(deciding))
      : "restricted",
    deciding,
    refusing: refusals.length,
    refused: refusals,
    recipients,
  };
}

/**
 * The status of a refusal a pressed Send came back with.
 *
 * A refusal whose reasons this build cannot read is still a refusal. It reads
 * as a stop with no cause named, never as a gap or as ready.
 */
export function refusedStatus(
  refusals: readonly RefusedRecipient[],
): SendStatus {
  return statusOf(refusals, 0);
}

/** The status of the preview answer. */
export function previewStatus(
  answer: Readonly<{
    preview: Preview | undefined;
    asking: boolean;
    unanswered: boolean;
  }>,
): SendStatus {
  const none = { refusing: 0, refused: [], recipients: 0 };
  if (answer.unanswered) return { kind: "unavailable", ...none };
  if (answer.asking) return { kind: "checking", ...none };
  const { preview } = answer;
  if (!preview) return { kind: "idle", ...none };
  // An answer about nobody answers nothing. The hook refuses one; a surface
  // handed a preview some other way must not read it as permission either.
  if (preview.recipients.length === 0) return { kind: "unavailable", ...none };
  const refusing = preview.recipients.filter((r) => r.would_refuse);
  if (refusing.length === 0) {
    return {
      kind: "ready",
      refusing: 0,
      refused: [],
      recipients: preview.recipients.length,
    };
  }
  return statusOf(refusing.map(refusalOf), preview.recipients.length);
}
