// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";

import { contactEditFields, mapContactUpdate } from "./contactformfields";

const t = (key: string) => key;

describe("the greeting register on a contact edit", () => {
  it("offers formal, familiar and let Margince decide, and not on create", () => {
    const field = contactEditFields(t).find(
      (candidate) => candidate.key === "greeting_register",
    );
    expect(field?.type).toBe("select");
    expect(
      field && "options" in field ? field.options?.map((o) => o.value) : [],
    ).toEqual(["", "formal", "familiar"]);
  });

  it("saves the chosen register", () => {
    expect(
      mapContactUpdate({ full_name: "Dana", greeting_register: "formal" })
        .greeting_register,
    ).toBe("formal");
    expect(
      mapContactUpdate({ full_name: "Dana", greeting_register: "familiar" })
        .greeting_register,
    ).toBe("familiar");
  });

  it("clears the register when the blank option is chosen", () => {
    expect(
      mapContactUpdate({ full_name: "Dana", greeting_register: "" })
        .greeting_register,
    ).toBeNull();
  });

  it("leaves the register alone when the form never held it", () => {
    expect(
      mapContactUpdate({ full_name: "Dana" }).greeting_register,
    ).toBeUndefined();
  });
});
