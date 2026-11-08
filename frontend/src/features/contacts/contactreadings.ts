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

// What an allowed answer has on record, by the reason the engine gave. It is
// keyed by reason, not purpose class. A marketing purpose allowed on the
// existing-customer route has no marketing permission on file. Saying so would
// claim a consent nobody gave.
const ALLOWED_FACT: Partial<Record<GuardEntry["reason_code"], MessageKey>> = {
  consent_confirmed: "contact.consent.fact.marketingPermission",
  existing_customer: "contact.consent.fact.existingCustomer",
  qualifying_event: "contact.consent.fact.businessContact",
  said_we_may_write: "contact.consent.fact.saidWeMayWrite",
  contract_notice: "contact.consent.fact.serviceMessages",
};

/**
 * The fact a contact's record holds for one purpose. The words cannot be read
 * as permission for every message: "Recent business contact recorded", not
 * "Allowed". Each message is still checked on its own when it is sent.
 *
 * A reason off a newer server falls to the plain answer for its verdict. A gap
 * then never reads as a recorded stop or a recorded grant.
 */
export function scopedConsentWord(
  entry: Pick<GuardEntry, "verdict" | "purpose_class" | "reason_code">,
  t: ReturnType<typeof useT>,
): string {
  const marketing = entry.purpose_class === "marketing";
  switch (entry.verdict) {
    case "allowed":
      return t(
        ALLOWED_FACT[entry.reason_code] ?? "contact.consent.allowedWord",
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
