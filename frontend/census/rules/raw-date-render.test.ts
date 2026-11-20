// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir, treeOf } from "../tree";
import { readingOf, rule } from "./raw-date-render.census";

// A planted screen over a real wire type. The checker resolves its field to
// the generated schema, as it does for a shipped screen.
const screen = (folder: string, body: string) => ({
  path: join(srcDir, "features", folder, "planted.tsx"),
  area: "src" as const,
  text: [
    'import type { components } from "../../api/schema";',
    'import { formatDate, formatDateAbbrev } from "../../format/format";',
    'type Rate = components["schemas"]["FxRate"];',
    'type Tax = components["schemas"]["TaxRate"];',
    "const labels = { effective_date: 'Effective' };",
    body,
  ].join("\n"),
});

const findings = (folder: string, body: string) =>
  plantedFindings(rule, [screen(folder, body)]);

describe("a date reaches the reader through a formatter", () => {
  it("reports a wire date shown as a child, a shown attribute and a column's render", () => {
    expect(
      findings(
        "deals",
        [
          "export const A = ({ row }: { row: Rate }) => <p>{row.effective_date}</p>;",
          "export const B = ({ row }: { row: Rate }) => <p title={row.effective_date}>x</p>;",
          "export const C = { render: (row: Rate) => row.effective_date };",
        ].join("\n"),
      ),
    ).toEqual([
      "src/features/deals/planted.tsx:6: row.effective_date is shown raw in a JSX child",
      "src/features/deals/planted.tsx:7: row.effective_date is shown raw in a shown attribute",
      "src/features/deals/planted.tsx:8: row.effective_date is shown raw in a column's render",
    ]);
  });

  it("sees a date field by its declared format, through the operators that pass it on", () => {
    expect(
      findings(
        "deals",
        [
          "export const A = ({ row }: { row: Tax }) => <p>{row.valid_from}</p>;",
          'export const B = ({ row }: { row?: Rate }) => <p>{row?.effective_date ?? "—"}</p>;',
          'export const C = ({ row, on }: { row: Rate; on: boolean }) => <p>{on ? (row.effective_date!) : "—"}</p>;',
          "export const D = { render: (row: Rate) => { return row.effective_date || null; } };",
        ].join("\n"),
      ),
    ).toEqual([
      "src/features/deals/planted.tsx:6: row.valid_from is shown raw in a JSX child",
      "src/features/deals/planted.tsx:7: row?.effective_date is shown raw in a JSX child",
      "src/features/deals/planted.tsx:8: row.effective_date is shown raw in a JSX child",
      "src/features/deals/planted.tsx:9: row.effective_date is shown raw in a column's render",
    ]);
  });

  it("counts the wire-date reads it saw, so a broken lookup fails its floor", () => {
    const tree = treeOf([
      screen(
        "deals",
        'export const A = ({ row }: { row: Tax }) => formatDate(row.valid_from, "en", "UTC");',
      ),
    ]);
    expect(
      readingOf(rule.corpus(tree), tree.checked().getTypeChecker()).wireDates,
    ).toBe(1);
  });

  it("passes a formatted date, a machine attribute and a local field of the same name", () => {
    expect(
      findings(
        "deals",
        [
          'export const A = ({ row }: { row: Rate }) => <p>{formatDate(row.effective_date, "en", "UTC")}</p>;',
          "export const B = ({ row }: { row: Rate }) => <time dateTime={row.effective_date}>x</time>;",
          "export const C = () => <p title={labels.effective_date}>x</p>;",
        ].join("\n"),
      ),
    ).toEqual([]);
  });

  it("reports the abbreviated day style in Settings alone", () => {
    const abbreviated =
      'export const A = ({ row }: { row: Rate }) => <p>{formatDateAbbrev(row.effective_date, "en", "UTC")}</p>;';
    expect(findings("settings", abbreviated)).toEqual([
      "src/features/settings/planted.tsx:6: formatDateAbbrev is a second day style in Settings",
    ]);
    expect(findings("deals", abbreviated)).toEqual([]);
  });
});
