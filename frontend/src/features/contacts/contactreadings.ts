import type { components } from "../../api/schema";
import type { useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { valueFor } from "../../lib/labelfor";

type GuardEntry = components["schemas"]["ContactConsentGuardEntry"];

// The reasons that record the contact saying stop, as opposed to a gap.
const STOP_REASONS: ReadonlySet<string> = new Set([
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
 * An unfamiliar reason keeps the purpose scope. A gap never reads as a
 * recorded stop or a recorded grant.
 */
export function scopedConsentWord(
  entry: Pick<GuardEntry, "verdict" | "purpose_class"> & {
    reason_code: string;
  },
  t: ReturnType<typeof useT>,
): string {
  const marketing = entry.purpose_class === "marketing";
  switch (entry.verdict) {
    case "allowed":
      return t(
        valueFor(ALLOWED_FACT, entry.reason_code, undefined) ??
          "contact.consent.fact.purposeSupported",
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
