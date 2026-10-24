// Unknown refusal codes retain the generic refusal instead of exposing a code.
// `recordable`: the surface draws a control to answer the refusal, so the sentence
// may ask the rep to use it. A read-only surface gets the fact alone.
export function sendPermissionReasonKey(
  reason: string | undefined,
  recordable = false,
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
      return recordable
        ? ("sendPermission.reason.noMarketingConsent" as const)
        : ("sendPermission.reason.noMarketingConsentFact" as const);
    case "legacy_transactional_unevidenced":
      return recordable
        ? ("sendPermission.reason.legacyPurpose" as const)
        : ("sendPermission.reason.legacyPurposeFact" as const);
    case "unknown_purpose":
      return "sendPermission.reason.unknownPurpose" as const;
    case "claim_contradicts_resolution":
      return "sendPermission.reason.claimContradicts" as const;
    default:
      return "sendPermission.reason.other" as const;
  }
}
