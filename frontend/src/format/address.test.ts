import { describe, expect, it } from "vitest";

import { countryBeyondCity } from "./address";

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
