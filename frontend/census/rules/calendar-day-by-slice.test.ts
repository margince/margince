// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, srcDir } from "../tree";
import {
  couldSliceAnIso,
  isoSliceSites,
  rule,
} from "./calendar-day-by-slice.census";

const keysIn = (text: string) =>
  isoSliceSites(
    parseSource(join(srcDir, "screens", "planted.tsx"), text),
    "screens/planted.tsx",
  ).map((finding) => finding.key);

describe("a calendar day is never cut out of an ISO string", () => {
  it("does not drop a file the detector would have matched", () => {
    expect(
      couldSliceAnIso("const day = new Date().toISOString().slice(0, 10);"),
    ).toBe(true);
    expect(couldSliceAnIso("const initials = name.slice(0, 2);")).toBe(false);
  });

  it("recognises the day cut and the month cut", () => {
    expect(
      keysIn(
        "export function today() { return new Date().toISOString().slice(0, 10); }",
      ),
    ).toEqual(["screens/planted.tsx#today"]);
    expect(
      keysIn(
        "export function thisMonth() { return new Date().toISOString().slice(0, 7); }",
      ),
    ).toEqual(["screens/planted.tsx#thisMonth"]);
  });

  it("passes a string that is not an ISO string, and the time half of one", () => {
    expect(
      keysIn(
        "export function initials(name: string) { return name.slice(0, 2); }",
      ),
    ).toEqual([]);
    expect(
      keysIn(
        "export function at(d: Date) { return d.toISOString().slice(11, 16); }",
      ),
    ).toEqual([]);
  });
});

describe("the calendar-day-by-slice rule", () => {
  it("reports an unratified cut and a ratified function whose count moved", () => {
    const findings = plantedFindings(rule, [
      {
        path: join(srcDir, "screens", "planted.tsx"),
        area: "src",
        text: "export function today() { return new Date().toISOString().slice(0, 10); }",
      },
      {
        path: join(srcDir, "mcp-apps", "values.ts"),
        area: "src",
        text: "export function day(d: Date) { return [d.toISOString().slice(0, 10), d.toISOString().slice(0, 10)]; }",
      },
    ]);
    expect(findings).toEqual([
      "screens/planted.tsx#today:1",
      "features/ai/aiusage.tsx#monthAround: ratified 2, found 0",
      "format/calendarday.ts#shifted: ratified 1, found 0",
      "mcp-apps/values.ts#day: ratified 1, found 2",
    ]);
  });
});
