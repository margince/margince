// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { en, type MessageKey } from "../i18n/en";
import { companyRefusal, companySaveMessage } from "./installationcompany";
import { ProblemError, problemMessageOf } from "./problem";

const t = (key: MessageKey) => en[key];

function refusal(field: string, code: string, extra: object = {}) {
  return new ProblemError({
    title: "Unprocessable Entity",
    status: 422,
    code: "validation_error",
    detail: "the request failed validation",
    details: {
      errors: [{ field, code, message: "at most 200 characters", ...extra }],
    },
  });
}

describe("companyRefusal", () => {
  it.each(["display_name", "profile.display_name"])(
    "puts a too-long %s on the name with the limit it names",
    (field) => {
      const { byField, rest } = companyRefusal(
        refusal(field, "too_long", { max: 200 }),
        t,
      );
      expect(byField.get("display_name")).toBe(
        "Company name: at most 200 characters.",
      );
      expect(rest).toBeNull();
    },
  );

  it("leaves a refusal of a field the form does not show at its foot", () => {
    const { byField, rest } = companyRefusal(
      refusal("profile.history", "too_long", { max: 4000 }),
      t,
    );
    expect(byField.size).toBe(0);
    expect(rest).toBe("This value is at most 4,000 characters.");
  });
});

describe("companySaveMessage", () => {
  it("names the name's limit where no field can carry it", () => {
    expect(
      companySaveMessage(
        refusal("profile.display_name", "too_long", { max: 200 }),
        t,
      ),
    ).toBe("Company name: at most 200 characters.");
  });

  it("keeps the shared words for a refusal no field names", () => {
    const other = new ProblemError({ status: 409, code: "version_skew" });
    expect(companySaveMessage(other, t)).toBe(problemMessageOf(other, t));
  });
});
