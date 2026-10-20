import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { LOCALES, translate } from "../../i18n";
import {
  commercialPreviewDisplay,
  LIFECYCLE_CONFLICT_LABELS,
} from "./approvalpreview";

function sourceFunction(path: string, name: string): string {
  const source = readFileSync(
    new URL(`../../../../backend/internal/compose/${path}`, import.meta.url),
    "utf8",
  );
  const start = source.indexOf(`func ${name}(`);
  expect(start).toBeGreaterThanOrEqual(0);
  const end = source.indexOf("\n}", start);
  expect(end).toBeGreaterThan(start);
  return source.slice(start, end);
}

describe("lifecycle conflict translations", () => {
  it("covers every reason emitted by the lifecycle reader", () => {
    const blocking = sourceFunction(
      "lifecyclereview/latest.go",
      "BlockingReason",
    );
    const returns = [...blocking.matchAll(/\breturn\s+([^\n]+)/g)].map(
      (match) => match[1].trim(),
    );
    expect(returns.length).toBeGreaterThan(1);
    for (const value of returns) expect(value).toMatch(/^"[a-z_]*"$/);
    const preview = sourceFunction(
      "approvalpreviewevidence.go",
      "setLifecyclePreviewContext",
    );
    const assignments = [...preview.matchAll(/\breason\s*=(?!=)\s*([^\n]+)/g)].map(
      (match) => match[1].trim(),
    );
    expect(assignments.length).toBeGreaterThan(0);
    for (const value of assignments) expect(value).toMatch(/^"[a-z_]+"$/);
    const kinds = [
      ...new Set(
        [...returns, ...assignments]
          .map((value) => value.slice(1, -1))
          .filter(Boolean),
      ),
    ];
    expect(Object.keys(LIFECYCLE_CONFLICT_LABELS).sort()).toEqual(kinds.sort());
    for (const locale of LOCALES) {
      for (const kind of kinds) {
        const key = LIFECYCLE_CONFLICT_LABELS[kind];
        const rows = commercialPreviewDisplay(
          {
            version: 1,
            state: "ready",
            reviewer_reason: "record_authority",
            changes: [],
            conflicts: [{ kind, summary: "", blocks_approval: true }],
          },
          (label, params) => translate(locale, label, params),
        );
        const value = rows.find(
          (row) => row.field === "preview_conflict_0",
        )?.value;
        expect(value).toBe(translate(locale, key));
        expect(value).toMatch(/\p{L}/u);
        expect(value).not.toBe(key);
      }
    }
  });
});
