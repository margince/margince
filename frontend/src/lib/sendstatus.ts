import type { components } from "../api/schema";
import {
  type RefusalKind,
  refusalKindOf,
} from "../format/sendpermissionreason";

// ONE ANSWER about the message being written, from the two places the server
// gives one: the preview asked while the rep writes, and the refusal a pressed
// Send came back with. The composer used to draw both, so a red refusal stayed
// on screen beside a later check that said the message may go.

type Preview = components["schemas"]["SendAuthorizationPreview"];

/** One refused recipient, as the server named it. */
export type RefusedRecipient = Readonly<{
  address: string;
  reasonCode: string;
  category?: string;
}>;

export type SendStatusKind =
  | "idle"
  | "checking"
  | "ready"
  | "unavailable"
  | RefusalKind;

export type SendStatus = Readonly<{
  kind: SendStatusKind;
  /** The refusal the status explains: the one the rep is least able to fix. */
  deciding?: RefusedRecipient;
  /** How many recipients refuse, the deciding one included. */
  refusing: number;
  /** How many recipients the answer covers; 0 where the server did not say. */
  recipients: number;
}>;

// The order a rep is told about refusals in. A stop nobody here can lift
// comes first, because fixing the others would still not send the message.
const SEVERITY: readonly RefusalKind[] = [
  "restricted",
  "missingPermission",
  "missingContext",
  "missingEvidence",
];

function severityOf(recipient: RefusedRecipient): number {
  return SEVERITY.indexOf(
    refusalKindOf(recipient.reasonCode, recipient.category),
  );
}

/**
 * The status of a refusal a pressed Send came back with.
 *
 * A refusal whose reasons this build could not read is still a refusal: it
 * reads as a stop with no cause named, never as a gap or as ready.
 */
export function refusedStatus(
  refusals: readonly RefusedRecipient[],
): SendStatus {
  const deciding = refusals.reduce<RefusedRecipient | undefined>(
    (worst, next) =>
      worst === undefined || severityOf(next) < severityOf(worst)
        ? next
        : worst,
    undefined,
  );
  return {
    kind: deciding
      ? refusalKindOf(deciding.reasonCode, deciding.category)
      : "restricted",
    deciding,
    refusing: refusals.length,
    recipients: 0,
  };
}

/**
 * The status of the preview answer. The caller picks `deciding` with the same
 * rule that decides whether an override is offered.
 */
export function previewStatus(
  answer: Readonly<{
    preview: Preview | undefined;
    asking: boolean;
    unanswered: boolean;
  }>,
  deciding: Preview["recipients"][number] | undefined,
): SendStatus {
  if (answer.unanswered)
    return { kind: "unavailable", refusing: 0, recipients: 0 };
  if (answer.asking) return { kind: "checking", refusing: 0, recipients: 0 };
  const { preview } = answer;
  if (!preview) return { kind: "idle", refusing: 0, recipients: 0 };
  // An answer about nobody answers nothing. The hook refuses one; a surface
  // handed a preview some other way must not read it as permission either.
  if (preview.recipients.length === 0) {
    return { kind: "unavailable", refusing: 0, recipients: 0 };
  }
  const recipients = preview.recipients.length;
  if (!deciding) return { kind: "ready", refusing: 0, recipients };
  const refused: RefusedRecipient = {
    address: deciding.address,
    reasonCode: deciding.reason_code,
    category: deciding.resolved_category,
  };
  return {
    kind: refusalKindOf(refused.reasonCode, refused.category),
    deciding: refused,
    refusing: preview.recipients.filter((r) => r.would_refuse).length,
    recipients,
  };
}
