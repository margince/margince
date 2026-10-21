import { expect, it } from "vitest";
import { formatDate } from "../../format/format";
import { UTC_ZONE } from "../../format/timezone";
import { LOCALES, translate } from "../../i18n";
import { commercialPreviewDisplay } from "./approvalpreview";

it.each(LOCALES)(
  "explains employment succession and date precision in %s",
  (locale) => {
    const rows = commercialPreviewDisplay(
      {
        version: 2,
        state: "ready",
        reviewer_reason: "record_authority",
        changes: [],
        details: [
          {
            field: "relationship_kind",
            value: { kind: "text", text: "employment" },
          },
          {
            field: "archive_primary_employer",
            value: { kind: "reference", text: "Remaining employer" },
          },
          {
            field: "relationship_started",
            value: { kind: "date", text: "2024-01-01" },
          },
          {
            field: "relationship_start_precision",
            value: { kind: "text", text: "month" },
          },
          {
            field: "relationship_end_precision",
            value: { kind: "text", text: "day" },
          },
        ],
        effects: [
          "archive_relationship_preserve_records",
          "archive_relationship_promote_employer",
        ],
      },
      (key, params) => translate(locale, key, params),
      locale,
      (value) => formatDate(value, locale, UTC_ZONE),
    );
    for (const [label, value] of [
      ["rel.kind", translate(locale, "rel.kind.employment")],
      ["approval.preview.newPrimaryEmployer", "Remaining employer"],
      ["rel.startedAt", formatDate("2024-01-01", locale, UTC_ZONE)],
      [
        "approval.preview.startPrecision",
        translate(locale, "approval.preview.precisionMonth"),
      ],
      [
        "approval.preview.endPrecision",
        translate(locale, "approval.preview.precisionDay"),
      ],
    ] as const)
      expect(
        rows.find((row) => row.label === translate(locale, label))?.value,
      ).toBe(value);
    expect(
      rows.find(
        (row) => row.field === "effect_archive_relationship_promote_employer",
      )?.value,
    ).toBe(translate(locale, "approval.preview.archivePromoteEmployer"));
    const none = commercialPreviewDisplay(
      {
        version: 2,
        state: "ready",
        reviewer_reason: "record_authority",
        changes: [],
        effects: ["archive_relationship_no_primary"],
      },
      (key, params) => translate(locale, key, params),
      locale,
    );
    expect(
      none.find((row) => row.field === "effect_archive_relationship_no_primary")
        ?.value,
    ).toBe(translate(locale, "approval.preview.archiveNoPrimary"));
  },
);
