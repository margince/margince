import { describe, expect, it } from "vitest";

import { countryBeyondCity, postalLines } from "./address";

describe("the country of an address", () => {
  it("is dropped when it names the city-state the city already names", () => {
    expect(countryBeyondCity("Monaco", "MC")).toBeUndefined();
    expect(countryBeyondCity("Singapore", "sg")).toBeUndefined();
    expect(countryBeyondCity("Monaco", "Monaco")).toBeUndefined();
    // Held against the code's name in every language, case and marks folded.
    expect(countryBeyondCity("Singapur", "SG")).toBeUndefined();
    expect(countryBeyondCity("monaco", "MC")).toBeUndefined();
  });

  it("is kept when city and country are two places", () => {
    expect(countryBeyondCity("Munich", "DE")).toBe("DE");
    expect(countryBeyondCity("Luxembourg", "BE")).toBe("BE");
    expect(countryBeyondCity(undefined, "DE")).toBe("DE");
    expect(countryBeyondCity("Munich", undefined)).toBeUndefined();
    expect(countryBeyondCity("Munich", "  ")).toBeUndefined();
  });
});

describe("an address written as postal lines", () => {
  it("puts the street first, the postal code before the city, then the country", () => {
    expect(
      postalLines({
        line1: "Kaiser-Friedrich-Allee 51",
        line2: "",
        postal_code: "52074",
        city: "Aachen",
        region: null,
        country: "DE",
      }),
    ).toBe("Kaiser-Friedrich-Allee 51\n52074 Aachen\nDE");
  });

  it("drops empty parts and a country that only names the city again", () => {
    expect(postalLines({ city: "Monaco", country: "MC" })).toBe("Monaco");
    expect(postalLines({ line1: "  ", region: "Bavaria" })).toBe("Bavaria");
  });

  it("is empty when no part is set", () => {
    expect(postalLines(null)).toBe("");
    expect(postalLines({ line1: " ", city: null })).toBe("");
  });
});
