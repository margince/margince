import { describe, expect, it } from "vitest";
import { isSquareMark } from "./squaremark";

describe("isSquareMark", () => {
  it.each([
    [200, 200, true],
    [250, 200, true],
    [200, 250, true],
    [251, 200, false],
    [700, 200, false],
    [0, 200, false],
    [200, 0, false],
  ])("reads %ix%i as square: %s", (width, height, square) => {
    expect(isSquareMark(width, height)).toBe(square);
  });
});
