// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { translate } from "../i18n";
import {
  salutationChoice,
  salutationField,
  salutationFilter,
} from "./personsalutation";

describe("explicit salutations", () => {
  it("keeps an unset choice, a clear and an explicit none distinct", () => {
    expect(salutationChoice({ full_name: "Mrs Dana Example" })).toBeUndefined();
    expect(salutationChoice({ salutation: "" })).toBeNull();
    expect(salutationChoice({ salutation: "none" })).toBe("none");
    expect(salutationChoice({ salutation: "ms" })).toBe("ms");
  });
  it("offers an unset form choice and a separate unset list filter", () => {
    const t = translate.bind(null, "en");
    const field = salutationField(t);
    expect(field.options.map((option) => option.value)).toEqual([
      "",
      "mr",
      "ms",
      "none",
    ]);
    expect(salutationFilter(t).options.map((option) => option.value)).toEqual([
      "unset",
      "mr",
      "ms",
      "none",
    ]);
  });
});
