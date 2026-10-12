import { describe, expect, it } from "vitest";
import { formatAge, formatInterval } from "./interval";

describe("intervals in words", () => {
  it("reads a configured interval in every whole unit it holds", () => {
    expect(formatInterval(86_400, "en")).toBe("1 day");
    expect(formatInterval(5_400, "en")).toBe("1 hour, 30 minutes");
    expect(formatInterval(30, "de")).toBe("30 Sekunden");
    expect(formatInterval(0, "en")).toBe("0 seconds");
  });

  it("reads a wait in its largest whole unit", () => {
    expect(formatAge(3_660, "en")).toBe("1 hour");
    expect(formatAge(45, "en")).toBe("45 seconds");
    expect(formatAge(0, "en")).toBe("0 seconds");
  });
});
