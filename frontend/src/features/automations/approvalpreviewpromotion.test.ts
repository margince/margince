import { expect, it } from "vitest";
import { LOCALES, translate } from "../../i18n";
import { commercialPreviewDisplay } from "../../lib/approvalpreview";

it.each(LOCALES)(
  "names the industry a promotion fills on the company in %s",
  (locale) => {
    const rows = commercialPreviewDisplay(
      {
        version: 2,
        state: "ready",
        reviewer_reason: "record_authority",
        changes: [],
        details: [
          {
            field: "promotion_company_industry",
            value: { kind: "text", text: "logistics_transport" },
          },
        ],
        effects: ["promote_fill_company_industry"],
      },
      (key, params) => translate(locale, key, params),
      locale,
    );
    const industry = rows.find(
      (row) =>
        row.label ===
        translate(locale, "approval.preview.promotionCompanyIndustry"),
    );
    expect(industry?.value).toBe(
      translate(locale, "industry.logisticsTransport"),
    );
    expect(
      rows.find((row) => row.field === "effect_promote_fill_company_industry")
        ?.value,
    ).toBe(translate(locale, "approval.preview.promotionFillIndustry"));
    expect(rows.some((row) => row.value === "logistics_transport")).toBe(false);
  },
);
