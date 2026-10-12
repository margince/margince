import { describe, expect, it } from "vitest";
import {
  isRecord,
  problemCode,
  problemDetails,
  problemExistingId,
} from "./problembody";

describe("reading a problem body", () => {
  it("does not take an array for a record", () => {
    expect(isRecord({})).toBe(true);
    expect(isRecord([])).toBe(false);
    expect(isRecord(null)).toBe(false);
  });

  it("reads the details object, and nothing that is not one", () => {
    expect(problemDetails({ code: "x", details: { a: 1 } })).toEqual({ a: 1 });
    expect(problemDetails({ code: "x", details: [1] })).toBeNull();
    expect(problemDetails({ code: "x" })).toBeNull();
    expect(problemDetails("<html>502</html>")).toBeNull();
  });

  it("reads the code only when it is a string", () => {
    expect(problemCode({ code: "version_skew" })).toBe("version_skew");
    expect(problemCode({ code: 409 })).toBeNull();
    expect(problemCode(undefined)).toBeNull();
  });

  it("names a duplicate only with both a code and an existing id", () => {
    expect(
      problemExistingId({
        code: "duplicate_email",
        details: { existing_id: "c1" },
      }),
    ).toEqual({ id: "c1", code: "duplicate_email" });
    expect(
      problemExistingId({
        code: "duplicate_email",
        details: { existing_id: "" },
      }),
    ).toBeNull();
    expect(problemExistingId({ details: { existing_id: "c1" } })).toBeNull();
  });
});
