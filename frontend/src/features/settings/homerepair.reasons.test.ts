import { readFileSync } from "node:fs";
import { expect, it } from "vitest";
import { LOCALES, translate } from "../../i18n";
import { HOME_REPAIR_REASON_LABELS } from "./homerepair";

it("explains every disposition the suggestion repair owner emits", () => {
  const source = readFileSync(
    new URL(
      "../../../../backend/internal/modules/deals/suggestionrepair.go",
      import.meta.url,
    ),
    "utf8",
  );
  const declarations = [
    ...source.matchAll(/\b(RepairReason\w+)\s*=\s*"([a-z_]+)"/g),
  ];
  expect(declarations.length).toBeGreaterThan(0);
  const reasons = new Map(declarations.map((match) => [match[1], match[2]]));
  const assignments = [
    ...source.matchAll(/\bReason:\s*([^,}\n]+)/g),
    ...source.matchAll(/\.Reason\s*=\s*([^\n]+)/g),
  ];
  expect(assignments.length).toBe(declarations.length);
  for (const assignment of assignments) {
    const name = assignment[1].trim();
    expect(reasons.has(name), `undeclared repair reason: ${name}`).toBe(true);
  }
  for (const reason of reasons.values()) {
    const key = HOME_REPAIR_REASON_LABELS[reason];
    expect(key, reason).toBeDefined();
    for (const locale of LOCALES) {
      const text = translate(locale, key);
      expect(text).not.toBe(key);
      expect(text).toMatch(/\p{L}/u);
    }
  }
});
