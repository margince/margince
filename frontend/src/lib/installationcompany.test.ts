// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { en, type MessageKey } from "../i18n/en";
import { companySaveMessage } from "./installationcompany";
import { ProblemError, problemMessageOf } from "./problem";

const t = (key: MessageKey) => en[key];

function refusal(field: string, code: string) {
  return new ProblemError({
    title: "Unprocessable Entity",
    status: 422,
    code: "validation_error",
    detail: "the request failed validation",
    details: { errors: [{ field, code, message: "at most 200 characters" }] },
  });
}

describe("companySaveMessage", () => {
  it.each(["display_name", "profile.display_name"])(
    "reads a too-long %s in the reader's words",
    (field) => {
      expect(companySaveMessage(refusal(field, "too_long"), t)).toBe(
        en["settings.companyNameTooLong"],
      );
    },
  );

  it("keeps the server's words for any other refusal", () => {
    const other = refusal("profile.icp", "too_long");
    expect(companySaveMessage(other, t)).toBe(problemMessageOf(other, t));
  });
});
