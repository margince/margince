import { expect, it } from "vitest";
import type { components } from "../../api/schema";
import { LOCALES, translate } from "../../i18n";
import { commercialPreviewDisplay } from "../../lib/approvalpreview";

const preview: components["schemas"]["ApprovalPreview"] = {
  version: 1,
  state: "ready",
  reviewer_reason: "record_authority",
  changes: [],
};

it.each(LOCALES)(
  "words a company status, not a deal status, in %s",
  (locale) => {
    const t = (key: Parameters<typeof translate>[1]) => translate(locale, key);
    const rows = commercialPreviewDisplay(
      {
        ...preview,
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
