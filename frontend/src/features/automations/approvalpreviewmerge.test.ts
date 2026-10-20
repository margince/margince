import { expect, it } from "vitest";
import type { components } from "../../api/schema";
import { LOCALES, translate } from "../../i18n";
import { commercialPreviewDisplay } from "./approvalpreview";

it.each(LOCALES)(
  "renders merge consequences and preserves the proposed content in %s",
  (locale) => {
    const preview: components["schemas"]["ApprovalPreview"] = {
      version: 2,
      state: "ready",
      reviewer_reason: "",
      changes: [],
      details: [
        { field: "merge_source", value: { kind: "text", text: "Mira Vogel" } },
        {
          field: "merge_survivor",
          value: { kind: "text", text: "Mira Becker" },
        },
        {
          field: "merge_introduction_after",
          value: { kind: "text", text: "name_drop_approved" },
        },
        {
          field: "merge_run_after",
          value: { kind: "text", text: "cancelled" },
        },
        {
          field: "merge_run_skip",
          value: { kind: "text", text: "category_in_flight" },
        },
        {
          field: "merge_company_fact_field",
          value: { kind: "text", text: "employee_range" },
        },
        { field: "merge_grant_after", value: { kind: "text", text: "write" } },
        {
          field: "merge_claim_quote",
          value: { kind: "text", text: "Please deliver on Thursday." },
        },
      ],
      effects: ["merge_records_release_source"],
    };
    const t = (key: Parameters<typeof translate>[1]) => translate(locale, key);
    const rows = commercialPreviewDisplay(preview, t, locale);
    const values = rows.map((row) => row.value);
    expect(values).toContain("Mira Vogel");
    expect(values).toContain("Mira Becker");
    expect(values).toContain("Please deliver on Thursday.");
    expect(values).toContain(t("contact.intro.stateNameDropApproved"));
    expect(values).toContain(t("approval.merge.value.cancelled"));
    expect(values).toContain(t("approval.merge.value.category_in_flight"));
    expect(values).toContain(t("co.factField.employee_range"));
    expect(values).toContain(t("share.access.write"));
    expect(values).toContain(t("approval.preview.mergeRecords"));
    expect(rows.find((row) => row.value === "Mira Vogel")?.label).toBe(
      t("approval.merge.source"),
    );
  },
);

it("refuses to guess the meaning of a newer merge run state", () => {
  const rows = commercialPreviewDisplay(
    {
      version: 2,
      state: "ready",
      reviewer_reason: "",
      changes: [],
      details: [
        {
          field: "merge_run_after",
          value: { kind: "text", text: "future_state" },
        },
      ],
    },
    (key) => translate("en", key),
  );
  expect(rows.map((row) => row.value)).toContain(
    translate("en", "approval.preview.unavailable"),
  );
  expect(rows.map((row) => row.value)).not.toContain("future_state");
});
