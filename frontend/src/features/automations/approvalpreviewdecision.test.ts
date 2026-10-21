import { expect, it } from "vitest";
import type { components } from "../../api/schema";
import { LOCALES, translate } from "../../i18n";
import { commercialPreviewDisplay } from "./approvalpreview";
import { approvalAcceptable } from "./approvalresponsibility";

type Preview = components["schemas"]["ApprovalPreview"];

it("permits only supported complete decision contexts", () => {
  for (const version of [0, 1, 2, 3]) {
    for (const state of ["ready", "incomplete", "unavailable", "stale"]) {
      for (const blocks_approval of [false, true]) {
        expect(
          approvalAcceptable({
            preview: { version, state, conflicts: [{ blocks_approval }] },
          }),
        ).toBe(
          (version === 1 || version === 2) &&
            state === "ready" &&
            !blocks_approval,
        );
      }
    }
  }
});

it.each(LOCALES)("states the confirmation-only effect in %s", (locale) => {
  const question = "Has the account review been completed?";
  const rows = commercialPreviewDisplay(
    {
      version: 2,
      state: "ready",
      reviewer_reason: "record_authority",
      changes: [],
      details: [
        {
          field: "confirmation_question",
          value: { kind: "text", text: question },
        },
        {
          field: "confirmation_reason",
          value: { kind: "text", text: "Annual account review" },
        },
      ],
      effects: ["record_confirmation_only"],
    },
    (key, params) => translate(locale, key, params),
    locale,
  );
  expect(
    rows.find(
      (row) =>
        row.label ===
        translate(locale, "approval.preview.confirmationQuestion"),
    )?.value,
  ).toBe(question);
  expect(
    rows.find(
      (row) =>
        row.label === translate(locale, "approval.preview.confirmationReason"),
    )?.value,
  ).toBe("Annual account review");
  expect(
    rows.some(
      (row) =>
        row.value === translate(locale, "approval.preview.confirmationOnly"),
    ),
  ).toBe(true);
});

it.each(LOCALES)(
  "explains why an import forecast cannot be approved in %s",
  (locale) => {
    const incomplete: Preview = {
      version: 2,
      reviewer_reason: "record_authority",
      state: "incomplete",
      changes: [],
      conflicts: [
        {
          kind: "import_scope_unavailable",
          summary: "English fallback",
          blocks_approval: true,
        },
      ],
    };
    const rows = commercialPreviewDisplay(
      incomplete,
      (key, params) => translate(locale, key, params),
      locale,
    );
    expect(approvalAcceptable({ preview: incomplete })).toBe(false);
    expect(
      rows.some(
        (row) =>
          row.value ===
          translate(locale, "approval.preview.importScopeUnavailable"),
      ),
    ).toBe(true);
    expect(rows.some((row) => row.value === "English fallback")).toBe(false);
  },
);

it.each(LOCALES)(
  "shows required and optional stage criteria in %s",
  (locale) => {
    const keys = [
      "requiredCriterionMet",
      "requiredCriterionUnmet",
      "optionalCriterionMet",
      "optionalCriterionUnmet",
    ] as const;
    const fields = [
      "stage_required_criterion_met",
      "stage_required_criterion_unmet",
      "stage_optional_criterion_met",
      "stage_optional_criterion_unmet",
    ];
    const rows = commercialPreviewDisplay(
      {
        version: 1,
        state: "ready",
        reviewer_reason: "record_authority",
        changes: [],
        details: fields.map((field) => ({
          field,
          value: { kind: "text", text: "Buyer accepted the terms" },
        })),
      },
      (key, params) => translate(locale, key, params),
      locale,
    );
    for (const key of keys) {
      expect(
        rows.some(
          (row) =>
            row.label === translate(locale, `approval.preview.${key}`) &&
            row.value === "Buyer accepted the terms",
        ),
      ).toBe(true);
    }
  },
);

it.each(LOCALES)(
  "shows the unchanged commercial context beside a stage decision in %s",
  (locale) => {
    const rows = commercialPreviewDisplay(
      {
        version: 2,
        state: "ready",
        reviewer_reason: "requested_decider",
        changes: [],
        details: [
          {
            field: "company_id",
            value: { kind: "reference", text: "Harbor account" },
          },
          { field: "owner_id", value: { kind: "owner", text: "Anna Weber" } },
          {
            field: "amount_minor",
            value: { kind: "money", text: "12500000", currency: "EUR" },
          },
        ],
      },
      (key, params) => translate(locale, key, params),
      locale,
    );
    expect(rows.some((row) => row.value === "Harbor account")).toBe(true);
    expect(rows.some((row) => row.value === "Anna Weber")).toBe(true);
    const amount = rows.find(
      (row) => row.label === translate(locale, "history.field.amount_minor"),
    );
    expect(amount?.value).toMatch(/125[.,\s]?000/);
    expect(amount?.value).toMatch(/€|EUR/);
    expect(amount?.comparison).toBeNull();
  },
);
