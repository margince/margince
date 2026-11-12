import { expect, test } from "vitest";

// A throwaway test for the repeat job: it fails one run in three.
test("demo: fails one run in three", () => {
  expect(Math.random()).toBeGreaterThan(1 / 3);
});
