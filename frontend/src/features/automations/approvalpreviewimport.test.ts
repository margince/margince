import { expect, it } from "vitest";
import type { components } from "../../api/schema";
import { LOCALES, translate } from "../../i18n";
import { commercialPreviewDisplay } from "../../lib/approvalpreview";

it.each(LOCALES)(
  "keeps every import row outcome and full field change visible in %s",
  (locale) => {
    const body = "The complete imported activity body. ".repeat(60);
    const preview: components["schemas"]["ApprovalPreview"] = {
      version: 2,
      state: "ready",
      changes: [],
      reviewer_reason: "requested_decider",
      import_rows: [
        {
          object: "activity",
          source_key: "meeting-one",
          line: 2,
          outcome: "update",
          target: { kind: "reference", text: "Buyer discussion" },
          changes: [
            {
              field: "body",
              before: { kind: "text", text: "Previous notes" },
              after: { kind: "text", text: body },
            },
          ],
          details: [
            {
              field: "import_link_row_company",
              value: { kind: "text", text: "company-one" },
            },
          ],
          effects: [],
        },
        {
          object: "company",
          source_key: "company-two",
          line: 3,
          outcome: "skip",
          changes: [],
          details: [],
          effects: [],
        },
      ],
    };
    const rows = commercialPreviewDisplay(
      preview,
      (key, params) => translate(locale, key, params),
      locale,
    );
    expect(
      rows.find((row) => row.field === "import_0_body")?.comparison,
    ).toEqual({
      from: "Previous notes",
      to: body,
    });
    expect(rows.find((row) => row.field === "import_1")?.value).toBe(
      translate(locale, "approval.import.skip"),
    );
    expect(rows.find((row) => row.value === "company-one")?.label).toContain(
      translate(locale, "approval.import.companyRow"),
    );
    expect(rows.find((row) => row.field === "import_0")?.label).toContain(
      "meeting-one",
    );
  },
);
