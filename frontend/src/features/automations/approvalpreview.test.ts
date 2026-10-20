import { describe, expect, it } from "vitest";
import type { components } from "../../api/schema";
import { formatDate, formatMoney } from "../../format/format";
import { LOCALES, translate } from "../../i18n";
import {
  commercialPreviewDisplay,
  editedCommercialPreview,
} from "./approvalpreview";
import { approvalAcceptable } from "./approvalresponsibility";

type Preview = components["schemas"]["ApprovalPreview"];
const preview: Preview = {
  version: 1,
  state: "ready",
  reviewer_reason: "record_authority",
  changes: [
    {
      field: "owner_id",
      before: { kind: "owner", text: "Ada" },
      after: { kind: "owner", text: "Ben" },
    },
  ],
};

describe("commercial approval preview", () => {
  it.each(LOCALES)("labels a structured companion value in %s", (locale) => {
    const value = '{"city":"Berlin"}';
    const rows = commercialPreviewDisplay(
      {
        ...preview,
        changes: [
          {
            field: "address",
            before: { kind: "empty", text: "" },
            after: { kind: "structured", text: value },
          },
        ],
      },
      (key, params) => translate(locale, key, params),
    );
    expect(rows[0].comparison).toEqual({
      from: translate(locale, "approval.preview.empty"),
      to: translate(locale, "approval.preview.structured", { value }),
    });
  });
  it.each(LOCALES)("names the exact owner change in %s", (locale) => {
    const rows = commercialPreviewDisplay(preview, (key, params) =>
      translate(locale, key, params),
    );
    expect(rows[0].comparison).toEqual({ from: "Ada", to: "Ben" });
    expect(rows[0].label).toBe(translate(locale, "history.field.owner_id"));
    expect(rows[1].value).toBe(translate(locale, "approval.preview.authority"));
    expect(approvalAcceptable({ preview })).toBe(true);
  });
  it.each(["stale", "unavailable", "incomplete", "future_state"])(
    "explains %s without showing old changes",
    (state) => {
      const incomplete = { ...preview, state };
      const rows = commercialPreviewDisplay(incomplete, (key, params) =>
        translate("en", key, params),
      );
      expect(rows).toHaveLength(1);
      expect(rows[0].comparison).toBeNull();
      expect(rows[0].value).not.toContain("Ada");
      expect(approvalAcceptable({ preview: incomplete })).toBe(false);
    },
  );
  it("refuses an unsupported preview version", () => {
    const newer = { ...preview, version: 2 };
    expect(approvalAcceptable({ preview: newer })).toBe(false);
    const rows = commercialPreviewDisplay(newer, (key, params) =>
      translate("en", key, params),
    );
    expect(rows[0].value).toBe(translate("en", "approval.preview.unavailable"));
  });
});

it.each(LOCALES)("translates the win reason in %s", (locale) => {
  const rows = commercialPreviewDisplay(
    {
      ...preview,
      changes: [
        {
          field: "won_without_contract_reason",
          before: { kind: "empty", text: "" },
          after: { kind: "text", text: "verbal" },
        },
      ],
    },
    (key, params) => translate(locale, key, params),
  );
  expect(rows[0].label).toBe(translate(locale, "deals.winReason"));
  expect(rows[0].comparison).toEqual({
    from: translate(locale, "approval.preview.empty"),
    to: translate(locale, "deals.winReasonVerbal"),
  });
});

it.each(LOCALES)("keeps blocking conflict context visible in %s", (locale) => {
  const conflict = {
    kind: "active_engagement",
    summary: "A separate support agreement remains active.",
    blocks_approval: true,
  };
  const blocked = { ...preview, state: "incomplete", conflicts: [conflict] };
  const rows = commercialPreviewDisplay(blocked, (key, params) =>
    translate(locale, key, params),
  );
  expect(rows.find((row) => row.value === conflict.summary)?.label).toBe(
    translate(locale, "approval.preview.conflict"),
  );
  expect(
    approvalAcceptable({ preview: { ...preview, conflicts: [conflict] } }),
  ).toBe(false);
  expect(
    approvalAcceptable({
      preview: {
        ...preview,
        conflicts: [{ ...conflict, blocks_approval: false }],
      },
    }),
  ).toBe(true);
});

it.each(LOCALES)("states the deal outcome in %s", (locale) => {
  const rows = commercialPreviewDisplay(
    {
      ...preview,
      changes: [
        {
          field: "status",
          before: { kind: "text", text: "open" },
          after: { kind: "text", text: "won" },
        },
      ],
    },
    (key, params) => translate(locale, key, params),
  );
  expect(rows[0].comparison).toEqual({
    from: translate(locale, "deals.statusOpen"),
    to: translate(locale, "deals.statusWon"),
  });
});

it.each(LOCALES)("does not promise execution-time values in %s", (locale) => {
  const rows = commercialPreviewDisplay(
    {
      ...preview,
      changes: [
        {
          field: "closed_at",
          before: { kind: "empty", text: "" },
          after: { kind: "execution", text: "" },
        },
      ],
    },
    (key, params) => translate(locale, key, params),
  );
  expect(rows[0].comparison).toEqual({
    from: translate(locale, "approval.preview.empty"),
    to: translate(locale, "approval.preview.execution"),
  });
});

it.each(LOCALES)("names the additional effects of a win in %s", (locale) => {
  const rows = commercialPreviewDisplay(
    {
      ...preview,
      effects: ["classify_correspondence", "start_project_delivery"],
    },
    (key, params) => translate(locale, key, params),
  );
  expect(
    rows.find((row) => row.field === "effect_classify_correspondence")?.value,
  ).toBe(translate(locale, "approval.preview.correspondence"));
  expect(
    rows.find((row) => row.field === "effect_start_project_delivery")?.value,
  ).toBe(translate(locale, "approval.preview.delivery"));
});

it.each(LOCALES)(
  "formats money with its own currency and locale in %s",
  (locale) => {
    for (const currency of ["EUR", "JPY", "VND"]) {
      const rows = commercialPreviewDisplay(
        {
          ...preview,
          changes: [
            {
              field: "amount_minor",
              before: { kind: "money", text: "250000", currency: "EUR" },
              after: { kind: "money", text: "300000", currency },
            },
          ],
        },
        (key, params) => translate(locale, key, params),
        locale,
      );
      expect(rows[0].comparison).toEqual({
        from: formatMoney(250000, "EUR", locale),
        to: formatMoney(300000, currency, locale),
      });
    }
  },
);

it("keeps an unsafe integer exact in its labelled minor units", () => {
  const rows = commercialPreviewDisplay(
    {
      ...preview,
      changes: [
        {
          field: "amount_minor",
          before: { kind: "empty", text: "" },
          after: {
            kind: "money",
            text: "12345678901234567890",
            currency: "EUR",
          },
        },
      ],
    },
    (key, params) => translate("en", key, params),
  );
  expect(rows[0].comparison?.to).toBe("12345678901234567890 EUR minor units");
});

it.each(LOCALES)(
  "does not infer review authority from an unknown reason in %s",
  (locale) => {
    const rows = commercialPreviewDisplay(
      { ...preview, reviewer_reason: "future_reason" },
      (key, params) => translate(locale, key, params),
    );
    expect(rows.at(-1)?.value).toBe(
      translate(locale, "approval.preview.authorityUnknown"),
    );
  },
);
it.each(LOCALES)(
  "labels a withdrawn owner name without exposing it in %s",
  (locale) => {
    const rows = commercialPreviewDisplay(
      {
        ...preview,
        changes: [
          {
            field: "owner_id",
            before: { kind: "unavailable_name", text: "" },
            after: { kind: "owner", text: "Ben" },
          },
        ],
      },
      (key, params) => translate(locale, key, params),
    );
    expect(rows[0].comparison).toEqual({
      from: translate(locale, "approval.preview.nameUnavailable"),
      to: "Ben",
    });
  },
);

it.each(LOCALES)(
  "formats preview dates with the card's record clock in %s",
  (locale) => {
    const day = "2026-09-15";
    const rows = commercialPreviewDisplay(
      {
        ...preview,
        changes: [
          {
            field: "expected_close_date",
            before: { kind: "empty", text: "" },
            after: { kind: "date", text: day },
          },
        ],
      },
      (key, params) => translate(locale, key, params),
      locale,
      (value) => formatDate(value, locale, "Europe/Berlin"),
    );
    expect(rows[0].comparison?.to).toBe(
      formatDate(day, locale, "Europe/Berlin"),
    );
  },
);

it("shows edited words and lifecycle choices without changing reference labels", () => {
  const original = {
    ...preview,
    changes: [
      ...preview.changes,
      {
        field: "lost_reason",
        before: { kind: "empty", text: "" },
        after: { kind: "text", text: "Old reason" },
      },
      {
        field: "lifecycle",
        before: { kind: "lifecycle", text: "customer" },
        after: { kind: "lifecycle", text: "former_customer" },
      },
    ],
  };
  const edited = editedCommercialPreview(original, {
    owner_id: "unresolved ID",
    lost_reason: "Corrected reason",
    proposed_lifecycle: "prospect",
  });
  expect(edited?.changes[0]).toEqual(original.changes[0]);
  expect(edited?.changes[1].after.text).toBe("Corrected reason");
  expect(edited?.changes[2].after.text).toBe("prospect");
  expect(original.changes[1].after.text).toBe("Old reason");
});
