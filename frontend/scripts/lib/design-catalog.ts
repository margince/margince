// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { join } from "node:path";

// The tables of src/design-system/README.md, read once for the two gates that
// hold the tree to them: catalog.test.ts and sidebar.test.ts.

// Found by their HEADINGS rather than by line number: a line number would be a
// second copy of the file's shape, and every edit to the prose above would move it.
export const CATALOG_HEADING = "## What this directory already gives you";
export const ROOTS_HEADING = "## Seeing them";

// The raw text under `heading`, up to the next heading of the same level. A
// name that appears only in the prose at the top is a mention, not an entry.
export function catalogSection(readme: string, heading: string): string {
  const lines = readme.split("\n");
  const start = lines.indexOf(heading);
  if (start < 0) {
    throw new Error(
      `the design-system README has no "${heading}" section — ` +
        "the catalog gates read it, so renaming it silently empties them",
    );
  }
  const rest = lines.slice(start + 1);
  const end = rest.findIndex((line) => line.startsWith("## "));
  return (end < 0 ? rest : rest.slice(0, end)).join("\n");
}

const ALIGNMENT_RULE = /^\|?(\s*:?-+:?\s*\|)*\s*:?-+:?\s*\|?\s*$/;
const BLANK_ROW = /^[\s|]*$/;

export type Table = { header: string; rows: string[][] };

// Every table in `section`. A header is the row an alignment rule follows, so
// one section may hold many.
export function tablesIn(section: string): Table[] {
  const lines = section.split("\n");
  const tables: Table[] = [];
  let current: Table | undefined;
  for (const [index, line] of lines.entries()) {
    if (ALIGNMENT_RULE.test(line)) continue;
    if (!line.startsWith("|")) {
      current = undefined;
      continue;
    }
    if (BLANK_ROW.test(line)) {
      throw new Error(`a blank table row follows "${lines[index - 1]}"`);
    }
    const cells = line.slice(1).replace(/\|$/, "").split("|");
    if (ALIGNMENT_RULE.test(lines[index + 1] ?? "")) {
      current = { header: cells[0].trim(), rows: [] };
      tables.push(current);
    } else {
      current?.rows.push(cells);
    }
  }
  return tables;
}

export function tableRows(section: string, header: string): string[][] {
  return tablesIn(section)
    .filter((table) => table.header === header)
    .flatMap((table) => table.rows);
}

export type DesignCatalog = {
  catalogTable: string;
  rootsSection: string;
  /** The sidebar roots, in the order the Root table lists them. */
  roots: string[];
  /** The `Components/` categories, in order. */
  categories: string[];
  /** The `Foundations/` topics, in order. */
  topics: string[];
};

export function readDesignCatalog(frontendRoot: string): DesignCatalog {
  const readme = readFileSync(
    join(frontendRoot, "src", "design-system", "README.md"),
    "utf8",
  );
  const rootsSection = catalogSection(readme, ROOTS_HEADING);
  const rootRows = tableRows(rootsSection, "Root");
  return {
    catalogTable: catalogSection(readme, CATALOG_HEADING),
    rootsSection,
    // One row carries two roots, so every backticked `Name/` in the first cell
    // counts rather than the cell itself.
    roots: rootRows.flatMap((cells) =>
      [...cells[0].matchAll(/`([^`/]+)\/`/g)].map((match) => match[1]),
    ),
    categories: tableRows(rootsSection, "Category").map((cells) =>
      cells[0].trim(),
    ),
    // The Foundations row names its topics, and nothing else, in backticks.
    topics: [
      ...(
        rootRows.find((cells) => cells[0].includes("`Foundations/`"))?.[1] ?? ""
      ).matchAll(/`([^`]+)`/g),
    ].map((match) => match[1]),
  };
}
