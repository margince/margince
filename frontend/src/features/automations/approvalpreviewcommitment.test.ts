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

it.each(LOCALES)("names a commitment's recipients in %s", (locale) => {
  const rows = commercialPreviewDisplay(
    {
      ...preview,
      changes: [
        {
          field: "summary",
          before: { kind: "empty", text: "" },
          after: { kind: "text", text: "Send the recording" },
        },
        {
          field: "recipients",
          before: { kind: "empty", text: "" },
          after: { kind: "text", text: "Ines Huber, Morgan Recipient" },
        },
      ],
    },
    (key, params) => translate(locale, key, params),
  );
  expect(rows.find((row) => row.field === "recipients")).toMatchObject({
    label: translate(locale, "worklist.recipients.label"),
    comparison: { to: "Ines Huber, Morgan Recipient" },
  });
  expect(rows.find((row) => row.field === "summary")?.label).toBe(
    translate(locale, "approval.field.step"),
  );
});

it.each([true, false])(
  "compares task reference identities (same=%s)",
  (same) => {
    const rows = commercialPreviewDisplay(
      {
        ...preview,
        changes: [
          {
            field: "existing_task_id",
            before: {
              kind: "reference",
              text: "Send the recording",
              entity_type: "activity",
              entity_id: "33333333-3333-4333-8333-333333333333",
            },
            after: {
              kind: "reference",
              text: "Send the recording",
              entity_type: "activity",
              entity_id: same
                ? "33333333-3333-4333-8333-333333333333"
                : "44444444-4444-4444-8444-444444444444",
            },
          },
        ],
      },
      (key, params) => translate("en", key, params),
    );
    expect(
      rows.find((row) => row.field === "existing_task_id")?.comparison,
    ).toEqual(
      same ? null : { from: "Send the recording", to: "Send the recording" },
    );
  },
);
