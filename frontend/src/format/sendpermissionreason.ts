// Unknown refusal codes retain the generic refusal instead of exposing a code.
export function sendPermissionReasonKey(reason: string | undefined) {
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
      return "sendPermission.reason.noEvidence" as const;
    case "no_marketing_consent":
      return "sendPermission.reason.noMarketingConsent" as const;
    case "legacy_transactional_unevidenced":
      return "sendPermission.reason.legacyPurpose" as const;
    case "unknown_purpose":
      return "sendPermission.reason.unknownPurpose" as const;
    case "claim_contradicts_resolution":
      return "sendPermission.reason.claimContradicts" as const;
    default:
      return "sendPermission.reason.other" as const;
  }
}
