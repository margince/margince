import { describe, expect, it } from "vitest";
import { de } from "../i18n/de";
import { en, type MessageKey } from "../i18n/en";
import { vi } from "../i18n/vi";
import { problemMessage } from "./problem";

const t = (key: MessageKey) => en[key];

function refusal(details?: Record<string, unknown>) {
  return {
    code: "role_cannot_capture",
    detail: "role cannot capture",
    ...(details ? { details } : {}),
  };
}

describe("a capture refusal from the server", () => {
  it.each([
    ["activity", en["connectors.roleCannotCapture.activity"]],
    ["contact", en["connectors.roleCannotCapture.contact"]],
    ["lead", en["connectors.roleCannotCapture.lead"]],
    ["seat", en["connectors.roleCannotCapture.seat"]],
  ])("names the missing %s", (missing, says) => {
    expect(problemMessage(refusal({ missing }), t)).toBe(says);
  });

  it("names no grant when an older server sends none, or one it does not know", () => {
    const fallback = en["connectors.roleCannotCapture"];
    expect(problemMessage(refusal(), t)).toBe(fallback);
    expect(problemMessage(refusal({ missing: "constructor" }), t)).toBe(
      fallback,
    );
    expect(problemMessage(refusal({ missing: 7 }), t)).toBe(fallback);
  });
});

it.each([en, de, vi])("localizes the contract merge refusal", (catalog) => {
  expect(
    problemMessage(
      {
        code: "validation_error",
        details: {
          errors: [
            {
              field: "target_id",
              code: "company_merge_contract_mismatch",
              message: "English server detail",
            },
          ],
        },
      },
      (key) => catalog[key],
    ),
  ).toBe(catalog["companies.contractMergeBlocked"]);
});

describe("a lead's refusals read as the product's own sentences", () => {
  const phoneFault = (code: string) => ({
    code: "validation_error",
    details: {
      errors: [{ field: "phone", code, message: "English server detail" }],
    },
  });

  const phoneCopy: [string, MessageKey][] = [
    ["phone_empty", "common.phone.empty"],
    ["phone_needs_country_code", "common.phone.needsCountryCode"],
    ["phone_malformed", "common.phone.malformed"],
  ];
  it.each(
    phoneCopy.flatMap(([code, key]) =>
      Object.entries({ en, de, vi }).map(
        ([locale, catalog]) => [code, locale, key, catalog] as const,
      ),
    ),
  )("reads a refused phone's %s in %s copy", (code, _locale, key, catalog) => {
    const says = problemMessage(
      phoneFault(code),
      (messageKey) => catalog[messageKey],
    );
    expect(says).toBe(catalog[key]);
  });

  it("names the deal that keeps a qualification from being reversed", () => {
    const says = problemMessage(
      {
        code: "contact_has_deal",
        detail:
          "the promoted contact is a stakeholder on a live deal; the promotion cannot be reversed",
      },
      t,
    );
    expect(says).toBe(en["lead.demoteContactHasDeal"]);
  });

  it("fits an edit of an existing lead as well as a create when a contact holds the address", () => {
    const says = problemMessage(
      { code: "contact_holds_email", detail: "English server detail" },
      t,
    );
    expect(says).not.toMatch(/no lead was created/);
    expect(says).toMatch(/open a deal on the contact/);
  });
});

// The server's timeout detail is English; every reader is told in their own words.
it.each([en, de, vi])(
  "localizes a request the server stopped for time",
  (catalog) => {
    expect(
      problemMessage(
        { code: "request_timed_out", detail: "English server detail" },
        (key) => catalog[key],
      ),
    ).toBe(catalog["common.requestTimedOut"]);
  },
);

describe("a currency rate refusal reads as the product's own sentence", () => {
  it.each([
    ["fx_rate_base_self", "from_currency", "settings.rates.fromIsBase"],
    ["fx_rate_currency", "from_currency", "settings.rates.fromNotCurrency"],
    ["fx_rate_positive", "rate", "settings.rates.rateNotPositive"],
    ["fx_rate_past", "effective_date", "settings.rates.pastDate"],
  ] as const)("words %s in every catalog", (code, field, key) => {
    for (const catalog of [en, de, vi]) {
      const problem = {
        code: "validation_error",
        details: {
          errors: [{ field, code, message: "English server detail" }],
        },
      };
      expect(problemMessage(problem, (each) => catalog[each])).toBe(
        catalog[key],
      );
    }
  });
});

describe("a name or membership clash", () => {
  it.each([
    ["pipeline_name_taken", "pipeline.nameTaken"],
    ["pipeline_default_taken", "pipeline.defaultTaken"],
    ["custom_field_label_taken", "cf.labelTaken"],
    ["already_on_list", "lists.alreadyOnShortlist"],
  ] as const)("words %s in each language", (code, key) => {
    for (const catalog of [en, de, vi]) {
      expect(
        problemMessage(
          { status: 409, code, detail: "an English API sentence" },
          (k: MessageKey) => catalog[k],
        ),
      ).toBe(catalog[key]);
    }
  });
});
