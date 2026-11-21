// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { addressFields, addressFrom } from "./companyform";

describe("the address country field", () => {
  it("offers countries by name in the reader's language and stores the code", () => {
    const country = addressFields("de").find(
      (field) => field.key === "address_country",
    );
    expect(country?.type).toBe("select");
    expect(country?.wireField).toBe("address.country");
    expect(country?.options).toContainEqual({
      value: "DE",
      label: "Deutschland",
    });
    const english = addressFields("en").find(
      (field) => field.key === "address_country",
    );
    expect(english?.options).toContainEqual({ value: "DE", label: "Germany" });
  });

  it("leaves the other five address columns as text", () => {
    const others = addressFields("en").filter(
      (field) => field.key !== "address_country",
    );
    expect(others).toHaveLength(5);
    expect(others.every((field) => field.type === undefined)).toBe(true);
  });

  it("prefills a stored lower-case code so the list can show it chosen", () => {
    expect(addressFrom({ country: "de" }).address_country).toBe("DE");
  });
});
