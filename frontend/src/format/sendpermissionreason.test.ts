// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { de } from "../i18n/de";
import { en } from "../i18n/en";
import { sendPermissionReasonKey } from "./sendpermissionreason";

// The server gives one reason when the contact has no open deal.
// It gives the same reason when an open deal exists but the recipient is no stakeholder.
describe("a deal follow-up the server found no evidence for", () => {
  it.each([
    ["recordable", { recordable: true }],
    ["read-only", {}],
    ["with a reason pick", { picksReason: true }],
  ])("reads the same true sentence on a %s surface", (_name, surface) => {
    const key = sendPermissionReasonKey(
      "no_compatible_evidence",
      surface,
      "active_deal_followup",
    );
    expect(key).toBe("sendPermission.reason.dealFact");
    expect(en[key]).toBe(
      "No open deal with this recipient supports this follow-up.",
    );
    expect(de[key]).not.toMatch(/Empfänger|Person|wähle/i);
  });

  it("keeps the generic sentence for any other category", () => {
    expect(
      sendPermissionReasonKey(
        "no_compatible_evidence",
        {},
        "requested_followup",
      ),
    ).toBe("sendPermission.unprovenRefuses");
  });
});
