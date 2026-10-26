/** What the surface draws, so a sentence asks only for a control that is there. */
export type RefusalSurface = Readonly<{
  /** A reason for contact can be recorded under the rep's name. */
  recordable?: boolean;
  /** The message's own reason for contact can be picked. */
  picksReason?: boolean;
}>;

// Unknown refusal codes retain the generic refusal instead of exposing a code.
// A read-only surface gets the fact alone.
export function sendPermissionReasonKey(
  reason: string | undefined,
  { recordable = false, picksReason = false }: RefusalSurface = {},
) {
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
    case "recipient_resolves_to_no_single_subject":
      return "sendPermission.reason.ambiguous" as const;
    case "unconfirmed_double_opt_in":
      return "sendPermission.reason.unconfirmed" as const;
    case "no_compatible_evidence":
      return recordable
        ? ("sendPermission.unprovenHint" as const)
        : ("sendPermission.unprovenRefuses" as const);
    case "no_marketing_consent":
      return picksReason
        ? ("sendPermission.reason.noMarketingConsent" as const)
        : ("sendPermission.reason.noMarketingConsentFact" as const);
    case "legacy_transactional_unevidenced":
      return picksReason
        ? ("sendPermission.reason.legacyPurpose" as const)
        : ("sendPermission.reason.legacyPurposeFact" as const);
    case "unknown_purpose":
      return picksReason
        ? ("sendPermission.reason.unknownPurpose" as const)
        : ("sendPermission.reason.unknownPurposeFact" as const);
    case "claim_contradicts_resolution":
      return picksReason
        ? ("sendPermission.reason.claimContradicts" as const)
        : ("sendPermission.reason.claimContradictsFact" as const);
    default:
      return "sendPermission.reason.other" as const;
  }
}
