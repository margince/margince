// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
import { describe, expect, it } from "vitest";
import { translate } from "../../i18n";
import { scopedConsentWord } from "./contactreadings";

// A contact's consent row names the fact on record, never a blanket
// "Allowed": a reader must not take one purpose's answer for every message.

const t = ((key: Parameters<typeof translate>[1]) =>
  translate("en", key)) as Parameters<typeof scopedConsentWord>[1];

type Entry = Parameters<typeof scopedConsentWord>[0];

describe("scopedConsentWord", () => {
  it.each<[string, Entry, string]>([
    [
      "correspondence on a recorded exchange",
      {
        verdict: "allowed",
        purpose_class: "business_correspondence",
        reason_code: "qualifying_event",
      },
      "Recent business contact recorded",
    ],
    [
      "marketing with consent on file",
      {
        verdict: "allowed",
        purpose_class: "marketing",
        reason_code: "consent_confirmed",
      },
      "Marketing permission recorded",
    ],
    [
      "marketing after an objection",
      {
        verdict: "blocked",
        purpose_class: "marketing",
        reason_code: "objection",
      },
      "Marketing opt-out recorded",
    ],
    [
      "marketing with no consent yet",
      {
        verdict: "blocked",
        purpose_class: "marketing",
        reason_code: "no_consent",
      },
      "No marketing permission recorded",
    ],
    [
      "an address that bounced",
      {
        verdict: "blocked",
        purpose_class: "business_correspondence",
        reason_code: "suppressed_hard_bounce",
      },
      "Address does not accept mail",
    ],
    [
      "correspondence after a stop",
      {
        verdict: "blocked",
        purpose_class: "business_correspondence",
        reason_code: "suppressed_objection",
      },
      "Stop recorded",
    ],
    [
      "nothing decided",
      {
        verdict: "unknown",
        purpose_class: "marketing",
        reason_code: "no_consent",
      },
      "Nothing recorded yet",
    ],
  ])("names %s", (_, entry, word) => {
    expect(scopedConsentWord(entry, t)).toBe(word);
  });

  // A newer server's purpose class must not read as a recorded grant.
  it("falls back to the plain word for a purpose class this build does not know", () => {
    const entry = {
      verdict: "allowed",
      purpose_class: "sms_outreach",
      reason_code: "consent_confirmed",
    } as unknown as Entry;
    expect(scopedConsentWord(entry, t)).toBe("Allowed");
  });
});
