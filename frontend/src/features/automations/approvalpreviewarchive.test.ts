import { expect, it } from "vitest";
import type { components } from "../../api/schema";
import { LOCALES, translate } from "../../i18n";
import { commercialPreviewDisplay } from "./approvalpreview";

type Preview = components["schemas"]["ApprovalPreview"];

it.each(LOCALES)("explains archive preservation in %s", (locale) => {
  for (const [effect, caption] of [
    ["archive_company_detach_scope", "approval.preview.archiveCompany"],
    ["archive_contact_detach_scope", "approval.preview.archiveContact"],
    ["archive_project_detach_scope", "approval.preview.archiveProject"],
    ["archive_deal_detach_scope", "approval.preview.archiveDeal"],
    ["archive_activity_preserve_content", "approval.preview.archiveActivity"],
    ["archive_product_preserve_snapshots", "approval.preview.archiveProduct"],
    ["archive_template_preserve_offers", "approval.preview.archiveTemplate"],
    ["archive_offer_preserve_lines", "approval.preview.archiveOffer"],
  ] as const) {
    const preview: Preview = {
      version: 2,
      state: "ready",
      reviewer_reason: "record_authority",
      changes: [],
      effects: [effect],
    };
    const rows = commercialPreviewDisplay(
      preview,
      (key, params) => translate(locale, key, params),
      locale,
    );
    expect(rows.find((row) => row.field === `effect_${effect}`)?.value).toBe(
      translate(locale, caption),
    );
  }
});

it.each(LOCALES)("identifies the offer revision and status in %s", (locale) => {
  for (const status of [
    "draft",
    "sent",
    "accepted",
    "rejected",
    "expired",
    "superseded",
  ] as const) {
    const rows = commercialPreviewDisplay(
      {
        version: 2,
        state: "ready",
        reviewer_reason: "record_authority",
        changes: [],
        details: [
          { field: "offer_number", value: { kind: "text", text: "O-1042" } },
          { field: "revision", value: { kind: "number", text: "3" } },
          { field: "offer_status", value: { kind: "text", text: status } },
        ],
      },
      (key, params) => translate(locale, key, params),
      locale,
    );
    expect(
      rows.find((row) => row.label === translate(locale, "deal.offerNumber"))
        ?.value,
    ).toBe("O-1042");
    expect(
      rows.find((row) => row.label === translate(locale, "deal.offerRevision"))
        ?.value,
    ).toBe("3");
    expect(
      rows.find(
        (row) => row.label === translate(locale, "history.field.status"),
      )?.value,
    ).toBe(translate(locale, `commercial.offer.${status}`));
  }
});

it.each(LOCALES)("names each removed record assignment in %s", (locale) => {
  const fields = [
    ["archive_email", "buyer@example.test", "approval.preview.archiveEmail"],
    ["archive_phone", "+493012345678", "approval.preview.archivePhone"],
    ["contact_id", "Dana Buyer", "search.kind.contact"],
    ["archive_domain", "example.test", "approval.preview.archiveDomain"],
    ["archive_company_type", "partner", "approval.preview.archiveCompanyType"],
    ["archive_partner", "Example", "approval.preview.archivePartner"],
    ["archive_channel", "telegram", "approval.preview.archiveChannel"],
    [
      "archive_stakeholder",
      "Review sponsor",
      "approval.preview.archiveStakeholder",
    ],
    ["archive_role", "Decision maker", "approval.preview.archiveRole"],
    ["archive_list", "Review shortlist", "approval.preview.archiveList"],
    ["archive_tag", "Review category", "approval.preview.archiveTag"],
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
    },
    (key, params) => translate(locale, key, params),
    locale,
  );
  for (const [field, value, key] of fields) {
    const expected =
      field === "archive_company_type"
        ? translate(locale, "company.relType.partner")
        : value;
    expect(
      rows.find((row) => row.label === translate(locale, key))?.value,
    ).toBe(expected);
  }
});
