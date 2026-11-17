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
  const phoneFault = (code: string, message: string) => ({
    code: "validation_error",
    details: { errors: [{ field: "phone", code, message }] },
  });

  it.each([en, de, vi])(
    "asks for the country code a phone lacks",
    (catalog) => {
      const says = problemMessage(
        phoneFault(
          "phone_needs_country_code",
          "the number needs its country prefix (+49…, 0049…)",
        ),
        (key) => catalog[key],
      );
      expect(says).toBe(catalog["common.phone.needsCountryCode"]);
    },
  );

  it.each([en, de, vi])("says what a malformed phone lacks", (catalog) => {
    const says = problemMessage(
      phoneFault("phone_malformed", "not an E.164 number (+ and 8–15 digits)"),
      (key) => catalog[key],
    );
    expect(says).toBe(catalog["common.phone.malformed"]);
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
