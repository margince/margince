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
