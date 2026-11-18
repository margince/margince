import type { MessageKey } from "../i18n/en";

/** What the surface draws, so a sentence asks only for a control that is there. */
export type RefusalSurface = Readonly<{
  /** A reason for contact can be recorded under the rep's name. */
  recordable?: boolean;
  /** The message's own reason for contact can be picked. */
  picksReason?: boolean;
  /** The message was sent with no reason for contact of its own. */
  unclaimed?: boolean;
}>;

/**
 * Which of the composer's states a refusal is. The title names the kind of
 * gap: a detail of the message, a fact on record, a permission, or a stop.
 */
export type RefusalKind =
  | "missingContext"
  | "missingEvidence"
  | "missingFooter"
  | "missingPermission"
  | "restricted";

// A follow-up claimed or resolved for a live deal found none it may rest on. Its
// cause is the missing deal on the message, not anything the recipient did.
const DEAL_FOLLOWUP = "active_deal_followup";

// An unknown code is a stop: a newer server's restriction must not read as a
// gap this build would invite the rep to fill.
export function refusalKindOf(
  reason: string | undefined,
  category?: string,
): RefusalKind {
  switch (reason) {
    case "no_compatible_evidence":
      return category === DEAL_FOLLOWUP ? "missingContext" : "missingEvidence";
    case "deal_not_requested":
      return "missingEvidence";
    case "unknown_purpose":
    case "claim_contradicts_resolution":
    case "legacy_transactional_unevidenced":
      return "missingContext";
    case "footer_incomplete":
      return "missingFooter";
    case "no_marketing_consent":
    case "unconfirmed_double_opt_in":
    case "marketing_consent_required":
    case "marketing_permission_required":
    case "lawful_basis_unconfirmed":
      return "missingPermission";
    default:
      return "restricted";
  }
}

// A refusal the message's own reason can lift asks for that reason only where
// the surface draws the pick; elsewhere it states the fact alone.
type AskOrFact = Readonly<{ ask: MessageKey; fact: MessageKey }>;
const PICKABLE: ReadonlyMap<string, AskOrFact> = new Map([
  [
    "no_marketing_consent",
    {
      ask: "sendPermission.reason.noMarketingConsent",
      fact: "sendPermission.reason.noMarketingConsentFact",
    },
  ],
  [
    "legacy_transactional_unevidenced",
    {
      ask: "sendPermission.reason.legacyPurpose",
      fact: "sendPermission.reason.legacyPurposeFact",
    },
  ],
  [
    "unknown_purpose",
    {
      ask: "sendPermission.reason.unknownPurpose",
      fact: "sendPermission.reason.unknownPurposeFact",
    },
  ],
  [
    "claim_contradicts_resolution",
    {
      ask: "sendPermission.reason.claimContradicts",
      fact: "sendPermission.reason.claimContradictsFact",
    },
  ],
]);

// With no reason given, the purpose the engine could not place is the missing
// reason itself, not one the message named.
const NO_REASON: AskOrFact = {
  ask: "sendPermission.reason.noReason",
  fact: "sendPermission.reason.noReasonFact",
};

// Unknown refusal codes retain the generic refusal instead of exposing a code.
// A read-only surface gets the fact alone.
export function sendPermissionReasonKey(
  reason: string | undefined,
  {
    recordable = false,
    picksReason = false,
    unclaimed = false,
  }: RefusalSurface = {},
  category?: string,
) {
  const dealFollowup = category === DEAL_FOLLOWUP;
  const pickable =
    reason === "unknown_purpose" && unclaimed
      ? NO_REASON
      : PICKABLE.get(reason ?? "");
  if (pickable) {
    return picksReason ? pickable.ask : pickable.fact;
  }
  switch (reason) {
    case "marketing_objection":
      return "sendPermission.reason.objected" as const;
    case "consent_withdrawn":
      return "sendPermission.reason.withdrawn" as const;
    case "processing_restricted":
      return "sendPermission.reason.restricted" as const;
    case "subject_request":
      return "sendPermission.reason.askedUsToStop" as const;
    case "hard_bounce":
      return "sendPermission.reason.bounced" as const;
    case "frequency_cap_reached":
      return "sendPermission.reason.tooMany" as const;
    case "opted_out_after_request":
      return "sendPermission.reason.optedOutAfterRequest" as const;
    case "marketing_permission_required":
      return "sendPermission.reason.permissionRequired" as const;
    case "marketing_consent_required":
      return "sendPermission.reason.consentRequired" as const;
    case "recipient_resolves_to_no_single_subject":
      return "sendPermission.reason.ambiguous" as const;
    case "unconfirmed_double_opt_in":
      return "sendPermission.reason.unconfirmed" as const;
    case "footer_incomplete":
      return "sendPermission.reason.footerIncomplete" as const;
    case "lawful_basis_unconfirmed":
      return "sendPermission.reason.lawfulBasisUnconfirmed" as const;
    case "no_compatible_evidence":
      return dealFollowup
        ? ("sendPermission.reason.chooseDeal" as const)
        : recordable
          ? ("sendPermission.unprovenHint" as const)
          : ("sendPermission.unprovenRefuses" as const);
    case "deal_not_requested":
      return "sendPermission.reason.dealNotRequested" as const;
    default:
      return "sendPermission.reason.other" as const;
  }
}
