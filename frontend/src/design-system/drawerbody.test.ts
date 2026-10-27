import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "vitest";

// jsdom does no layout, so pin the rule that keeps a scrolling drawer's cards
// from shrinking (#9248).
test("a banded drawer's form-stack body keeps its cards at full height", () => {
  const css = readFileSync(join(__dirname, "atoms.css"), "utf8");
  expect(css).toMatch(
    /\.modal-drawer:has\(> \.drawer-head\) > \.drawer-body\.form-stack > \* \{\s*flex-shrink: 0;/,
  );
});
