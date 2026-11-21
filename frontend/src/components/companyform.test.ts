// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { addressFields, addressFrom, addressPatch } from "./companyform";

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

describe("a country stored as a name before the code rule", () => {
  const legacy = { country: "Germany", city: "Berlin" };

  it("is prefilled as stored and sent back as stored on a city-only edit", () => {
    const values: Record<string, string> = {
      ...addressFrom(legacy),
      address_city: "Hamburg",
    };
    expect(values.address_country).toBe("Germany");
    expect(addressPatch(values)?.country).toBe("Germany");
  });

  it("is offered as the current value so the list does not read blank", () => {
    const country = addressFields("en", "Germany").find(
      (field) => field.key === "address_country",
    );
    expect(country?.options).toContainEqual({
      value: "Germany",
      label: "Germany",
    });
    const coded = addressFields("en", "de").find(
      (field) => field.key === "address_country",
    );
    expect(coded?.options?.some((option) => option.value === "de")).toBe(false);
  });

  it("is replaced by the code the user picks", () => {
    expect(
      addressPatch({ ...addressFrom(legacy), address_country: "de" })?.country,
    ).toBe("DE");
  });
});
