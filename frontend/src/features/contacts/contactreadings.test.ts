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
  // Every reason the engine gives an allowed answer, each with its own words.
  it.each<[Entry["reason_code"], Entry["purpose_class"], string]>([
    ["consent_confirmed", "marketing", "Marketing permission recorded"],
    ["existing_customer", "marketing", "Existing customer, marketing allowed"],
    [
      "qualifying_event",
      "business_correspondence",
      "Recent business contact recorded",
    ],
    [
      "said_we_may_write",
      "business_correspondence",
      "Permission to write recorded",
    ],
    ["contract_notice", "transactional", "Service messages allowed"],
  ])("names an allowed %s answer", (reason_code, purpose_class, word) => {
    expect(
      scopedConsentWord({ verdict: "allowed", purpose_class, reason_code }, t),
    ).toBe(word);
  });

  it.each<[string, Entry, string]>([
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

  // A newer server's reason must not read as a recorded grant.
  it("falls back to the plain word for a reason this build does not know", () => {
    const entry = {
      verdict: "allowed",
      purpose_class: "marketing",
      reason_code: "statutory_route",
    } as unknown as Entry;
    expect(scopedConsentWord(entry, t)).toBe("Allowed");
  });
});
