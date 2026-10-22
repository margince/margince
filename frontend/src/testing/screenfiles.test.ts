import { basename } from "node:path";
import { describe, expect, it } from "vitest";
import { screenFiles } from "./screenfiles";

describe("the screen census the guards read", () => {
  it("finds the whole screen tree, not a part of it", () => {
    const files = screenFiles();
    // A floor, so a walk that lost a folder fails here before any guard
    // reports a pass over a smaller tree.
    expect(files.length).toBeGreaterThan(500);
    expect(files.map((file) => basename(file))).toContain("contacts.tsx");
    expect(files.some((file) => file.includes("/deal360/"))).toBe(true);
  });

  it("leaves out tests, testkits and stories", () => {
    expect(
      screenFiles().filter((file) =>
        /\.(test|stories|testkit)\.tsx$/.test(file),
      ),
    ).toEqual([]);
  });
});
