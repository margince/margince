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

it.each(LOCALES)(
  "words a company status, not a deal status, in %s",
  (locale) => {
    const t = (key: Parameters<typeof translate>[1]) => translate(locale, key);
    const rows = commercialPreviewDisplay(
      {
        version: 1,
        state: "ready",
        reviewer_reason: "record_authority",
        changes: [
          {
            field: "status",
            before: { kind: "lifecycle", text: "customer" },
            after: { kind: "lifecycle", text: "former_customer" },
          },
        ],
      },
      (key, params) => translate(locale, key, params),
      locale,
    );
    expect(rows[0].comparison).toEqual({
      from: t("company.lifecycle.customer"),
      to: t("company.lifecycle.former_customer"),
    });
  },
);
