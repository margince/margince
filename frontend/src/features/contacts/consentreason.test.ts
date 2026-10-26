// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { viewerZone } from "../../format/timezone";
import { type Translator, translate } from "../../i18n";
import { en } from "../../i18n/en";
import { vi } from "../../i18n/vi";
import { consentPurposeName, consentReason } from "./consentreason";

type GuardEntry = components["schemas"]["ContactConsentGuardEntry"];
type ReasonCode = GuardEntry["reason_code"];

// Keyed by the contract's enum, so a code added there without a case here
// fails the typecheck rather than slipping past this census.
const EVERY_CODE: Record<ReasonCode, true> = {
  objection: true,
  suppressed_objection: true,
  suppressed_restricted: true,
  suppressed_subject_request: true,
  suppressed_hard_bounce: true,
  suppressed: true,
  contract_notice: true,
  no_call_path: true,
  said_we_may_write: true,
  unconfirmed_double_opt_in: true,
  no_connection: true,
  qualifying_event: true,
  consent_confirmed: true,
  consent_withdrawn: true,
  no_consent_exception_not_offered: true,
  no_consent: true,
};

const tVi: Translator = (key, params) => translate("vi", key, params);

function entry(over: Partial<GuardEntry>): GuardEntry {
  return {
    purpose_key: "business_correspondence",
    purpose_label: "Business correspondence",
    purpose_class: "business_correspondence",
    channel: "email",
    verdict: "allowed",
    reason: "the server's English sentence",
    reason_code: "no_consent",
    ...over,
  };
}

describe("the consent panel in the reader's language", () => {
  it.each(Object.keys(EVERY_CODE) as ReasonCode[])(
    "says %s in Vietnamese, not as the server's English",
    (code) => {
      const said = consentReason(
        entry({ reason_code: code }),
        tVi,
        "vi",
        viewerZone(),
      );
      expect(said).not.toBe("the server's English sentence");
      expect(Object.values(vi)).toContain(said);
      expect(Object.values(en)).not.toContain(said);
    },
  );

  it("dates an exchange that makes the mail lawful", () => {
    const said = consentReason(
      entry({
        reason_code: "qualifying_event",
        qualifying_event: {
          kind: "inbound_message",
          occurred_at: "2026-06-03T09:00:00Z",
          source_entity_type: "activity",
          source_entity_id: "a-1",
        },
      }),
      tVi,
      "vi",
      viewerZone(),
    );
    expect(said).toMatch(/^Họ đã viết cho bạn ngày /);
    expect(said).not.toContain("{date}");
  });

  it("names a seeded purpose in Vietnamese and keeps an operator's own name", () => {
    expect(consentPurposeName(entry({}), tVi)).toBe("Thư công việc");
    expect(
      consentPurposeName(
        entry({ purpose_key: "events", purpose_label: "Event invites" }),
        tVi,
      ),
    ).toBe("Event invites");
  });

  it("falls back to the server's sentence for a code this build does not know", () => {
    const fromNewerServer = Object.assign(entry({}), {
      reason_code: "a_newer_code",
    });
    expect(consentReason(fromNewerServer, tVi, "vi", viewerZone())).toBe(
      "the server's English sentence",
    );
  });
});
