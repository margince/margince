import { expect, it } from "vitest";
import { LOCALES, translate } from "../../i18n";
import { commercialPreviewDisplay } from "../../lib/approvalpreview";

it.each(LOCALES)(
  "shows the partner set-up terms and the unset margin tier in %s",
  (locale) => {
    const fields = [
      ["partner_kinds", "consulting,referral", "partner.kinds"],
      ["partner_stage", "contacted", "partner.stage"],
      ["partner_next_step", "Send the terms", "partner.nextStep"],
      ["partner_next_step_due_at", "2026-11-30", "partner.nextStepDue"],
    ] as const;
    const rows = commercialPreviewDisplay(
      {
        version: 2,
        state: "ready",
        reviewer_reason: "record_authority",
        changes: [],
        details: fields.map(([field, text]) => ({
          field,
          value: { kind: "text", text },
        })),
        effects: ["partner_margin_tier_unset"],
      },
      (key, params) => translate(locale, key, params),
      locale,
    );
    for (const [, value, key] of fields) {
      expect(rows.find((row) => row.value === value)?.label).toBe(
        translate(locale, key),
      );
    }
    expect(
      rows.find((row) => row.field === "effect_partner_margin_tier_unset")
        ?.value,
    ).toBe(translate(locale, "approval.preview.partnerTierUnset"));
  },
);
