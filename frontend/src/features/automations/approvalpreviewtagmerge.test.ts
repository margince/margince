import { expect, it } from "vitest";
import { LOCALES, translate } from "../../i18n";
import { commercialPreviewDisplay } from "./approvalpreview";

it.each(LOCALES)(
  "distinguishes moved and duplicate tag assignments in %s",
  (locale) => {
    const fields = [
      [
        "merge_tag_source",
        "Retiring category",
        "approval.preview.mergeTagSource",
      ],
      [
        "merge_tag_target",
        "Surviving category",
        "approval.preview.mergeTagTarget",
      ],
      ["merge_tag_move", "First account", "approval.preview.mergeTagMove"],
      [
        "merge_tag_collapse",
        "Second account",
        "approval.preview.mergeTagCollapse",
      ],
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
        effects: ["merge_tags_release_source"],
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
      rows.find((row) => row.field === "effect_merge_tags_release_source")
        ?.value,
    ).toBe(translate(locale, "approval.preview.mergeTags"));
  },
);
