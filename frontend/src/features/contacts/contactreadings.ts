import type { components } from "../../api/schema";
import type { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

type ConsentVerdict =
  components["schemas"]["ContactConsentGuardEntry"]["verdict"];

// The word the rail's consent rows put on a channel's verdict, read from the
// SERVER's key rather than the rendered word: a translated label must not
// change how the slot is coloured.
//
// A table rather than a switch, so a verdict the contract gains and this does
// not fails the BUILD. The `??` below is the same question asked of the WIRE,
// where the type is only a claim: a cached page talking to a newer API is
// handed a verdict this build has no word for, and an unmatched key would put
// an empty cell where a permission belongs.
const VERDICT_WORD: Record<ConsentVerdict, MessageKey> = {
  allowed: "contact.consent.allowedWord",
  blocked: "contact.consent.blockedWord",
  unknown: "contact.consent.unknownWord",
};

export function consentWord(
  verdict: ConsentVerdict | undefined,
  t: ReturnType<typeof useT>,
): string {
  // No entry, and a verdict off a newer server, land on the same word: in both
  // this build has no recorded answer to show.
  return t((verdict && VERDICT_WORD[verdict]) ?? "contact.consent.unknownWord");
}

type GuardEntry = components["schemas"]["ContactConsentGuardEntry"];

// The reasons that record the contact saying stop, as opposed to a gap.
const STOP_REASONS: ReadonlySet<GuardEntry["reason_code"]> = new Set([
  "objection",
  "suppressed_objection",
  "consent_withdrawn",
  "suppressed_subject_request",
  "suppressed_restricted",
  "suppressed",
]);

// What an allowed purpose has on record, by the class of message it covers.
const ALLOWED_FACT: Record<GuardEntry["purpose_class"], MessageKey> = {
  business_correspondence: "contact.consent.fact.businessContact",
  transactional: "contact.consent.fact.serviceMessages",
  marketing: "contact.consent.fact.marketingPermission",
  phone_outreach: "contact.consent.fact.calls",
};

/**
 * The fact a contact's record holds for one purpose, in words that cannot be
 * read as permission for every message: "Recent business contact recorded",
 * not "Allowed". Each message is still checked on its own when it is sent.
 *
 * A purpose class or reason off a newer server falls to the plain answer for
 * its verdict, so a gap never reads as a recorded stop or a recorded grant.
 */
export function scopedConsentWord(
  entry: Pick<GuardEntry, "verdict" | "purpose_class" | "reason_code">,
  t: ReturnType<typeof useT>,
): string {
  const marketing = entry.purpose_class === "marketing";
  switch (entry.verdict) {
    case "allowed":
      return t(
        ALLOWED_FACT[entry.purpose_class] ?? "contact.consent.allowedWord",
      );
    case "blocked":
      if (entry.reason_code === "suppressed_hard_bounce") {
        return t("contact.consent.fact.bounced");
      }
      if (STOP_REASONS.has(entry.reason_code)) {
        return t(
          marketing
            ? "contact.consent.fact.marketingOptOut"
            : "contact.consent.fact.stopped",
        );
      }
      return t(
        marketing
          ? "contact.consent.fact.noMarketingPermission"
          : "contact.consent.fact.notAllowed",
      );
    default:
      return t("contact.consent.fact.nothingRecorded");
  }
}
