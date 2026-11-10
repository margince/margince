// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { expect, it } from "vitest";
import { MAX_ACTIVITY_LINKS } from "./activitybody";

const CONTRACT = readFileSync(
  new URL("../../../backend/api/crm.yaml", import.meta.url),
  "utf8",
).split("\n");

// The `maxItems` of one schema's top-level `links` property, read by indent:
// a schema name sits at four spaces and its properties at eight.
function linksCap(schema: string): number {
  const start = CONTRACT.indexOf(`    ${schema}:`);
  if (start < 0) throw new Error(`crm.yaml has no schema ${schema}`);
  const end = CONTRACT.findIndex(
    (line, index) => index > start && /^ {4}\S/.test(line),
  );
  const block = CONTRACT.slice(start + 1, end < 0 ? undefined : end);
  const links = block.indexOf("        links:");
  if (links < 0) throw new Error(`${schema} has no links property`);
  for (const line of block.slice(links + 1)) {
    if (/^ {8}\S/.test(line)) break;
    const cap = /^ {10}maxItems: (\d+)$/.exec(line);
    if (cap) return Number(cap[1]);
  }
  throw new Error(`${schema}.links states no maxItems`);
}

it.each(["SendCompanyEmailRequest", "PreviewAccountSendRequest"])(
  "matches the maxItems of %s.links",
  (schema) => {
    expect(MAX_ACTIVITY_LINKS).toBe(linksCap(schema));
  },
);
