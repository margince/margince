import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";
import { LOCALES, translate } from "../../i18n";
import {
  commercialPreviewDisplay,
  LIFECYCLE_CONFLICT_LABELS,
} from "../../lib/approvalpreview";

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
    const assignments = [
      ...preview.matchAll(/\breason\s*=(?!=)\s*([^\n]+)/g),
    ].map((match) => match[1].trim());
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

it("keeps source date and scope visible while an active agreement blocks termination", () => {
  const rows = commercialPreviewDisplay(
    {
      version: 1,
      changes: [
        {
          field: "owner_id",
          before: { kind: "owner", text: "Ada" },
          after: { kind: "owner", text: "Ben" },
        },
      ],
      reviewer_reason: "record_authority",
      state: "stale",
      lifecycle_evidence: {
        scope: "workstream",
        occurred_at: "2026-07-31T12:00:00Z",
        occurred_precision: "instant",
        workstream: "Atlas pilot",
      },
      conflicts: [
        {
          kind: "active_contract",
          summary: "Server fallback",
          blocks_approval: true,
        },
      ],
    },
    (key) => key,
    "en",
    undefined,
    "UTC",
  );
  expect(rows).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        field: "lifecycle_scope",
        value: "approval.preview.scopeWorkstream",
      }),
      expect.objectContaining({
        field: "source_date",
        value: expect.stringContaining("31"),
      }),
      expect.objectContaining({ field: "workstream", value: "Atlas pilot" }),
      expect.objectContaining({
        field: "preview_conflict_0",
        value: "approval.preview.activeContract",
      }),
    ]),
  );
  expect(rows.some((row) => row.field === "owner_id")).toBe(false);
});

// An instant source is dated on the record's clock, so it does not follow the
// reader west of it. A `day` source keeps its own date, though in Auckland its
// stored noon is the next morning.
it("dates the lifecycle source on the record's clock", () => {
  const sourceDate = (
    occurred_at: string,
    occurred_precision: "instant" | "day",
    zone: string,
  ) =>
    commercialPreviewDisplay(
      {
        version: 1,
        changes: [],
        reviewer_reason: "record_authority",
        state: "ready",
        lifecycle_evidence: {
          scope: "company",
          occurred_at,
          occurred_precision,
        },
      },
      (key) => key,
      "en",
      undefined,
      zone,
    ).find((row) => row.field === "source_date")?.value;
  const instant = sourceDate(
    "2026-07-31T05:00:00Z",
    "instant",
    "Asia/Ho_Chi_Minh",
  );
  expect(instant).toContain("31");
  expect(instant).not.toContain("30");
  const day = sourceDate("2026-07-31T12:00:00Z", "day", "Pacific/Auckland");
  expect(day).toContain("31");
  expect(day).not.toContain("Aug");
});
