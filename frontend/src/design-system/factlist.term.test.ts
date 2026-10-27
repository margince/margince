import { readFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "vitest";

// jsdom does no layout, so pin the rule that wraps a one-word term in its column.
test("a fact's term wraps a long single word instead of overflowing", () => {
  const css = readFileSync(join(__dirname, "factlist.css"), "utf8");
  expect(css).toMatch(/\.factlist-term \{[^}]*overflow-wrap: anywhere;/);
});
