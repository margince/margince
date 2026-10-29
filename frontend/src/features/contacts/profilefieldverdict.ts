// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { unwrap } from "../../lib/problem";

type ProfileField = components["schemas"]["ContactProfileField"];
export type ProfileFieldVerdict =
  components["schemas"]["AIFeedbackInput"]["verdict"];

// The value the reader had in front of them when they decided. The ledger
// applies the verdict only while the field still holds that value.
export type ShownValue = Readonly<{ value: string; capturedAt: string }>;

export function shownValueOf(field: ProfileField): ShownValue {
  return { value: field.value, capturedAt: field.captured_at };
}

export async function recordProfileFieldVerdict({
  contactId,
  field,
  shown,
  verdict,
  correctedValue,
}: Readonly<{
  contactId: string;
  field: Pick<ProfileField, "field" | "claim_key">;
  shown: ShownValue;
  verdict: ProfileFieldVerdict;
  correctedValue?: string;
}>): Promise<void> {
  unwrap(
    await api.POST("/ai/feedback", {
      body: {
        subject_type: "contact",
        subject_id: contactId,
        claim_kind: "profile_field",
        // The server's own key, echoed back: a path spelled here would file the
        // verdict against a claim nothing reads.
        claim_path: field.claim_key ?? `profile_field:${field.field}`,
        verdict,
        corrected_value: correctedValue,
        value_shown: shown.value,
        value_captured_at: shown.capturedAt,
      },
    }),
  );
}
