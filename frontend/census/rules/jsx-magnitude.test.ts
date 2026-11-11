// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// biome-ignore-all lint/suspicious/noTemplateCurlyInString: these strings are
// planted source, and the ${...} in them is the subject under test.

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plant } from "../../scripts/lib/jsx-magnitude.sweep";
import { stable } from "../../src/format/collate";
import { srcDir, treeOf } from "../tree";
import {
  blindStringifiers,
  judge,
  rule,
  svgGeometry,
} from "./jsx-magnitude.census";

const blind = blindStringifiers();
const geometry = svgGeometry();

describe("a magnitude is drawn in the reader's own notation", () => {
  it("derives the stringifiers that answer in nobody's notation", () => {
    expect([...blind].sort(stable)).toEqual([
      "toExponential",
      "toFixed",
      "toPrecision",
      "toString",
    ]);
    // The locale-aware member is one-locale's subject.
    expect(blind.has("toLocaleString")).toBe(false);
  });

  it("derives the geometry it must not mistake for text", () => {
    expect(geometry.has("viewBox")).toBe(true);
    expect(geometry.has("d")).toBe(true);
    expect(geometry.size).toBeGreaterThan(100);
    // Own members only, so the text attributes on an svg stay gated.
    expect(geometry.has("aria-label")).toBe(false);
    expect(geometry.has("title")).toBe(false);
  });

  it("sees each shape of the defect, including the ones a grep cannot", () => {
    // Each line states its own verdict, so no list of line numbers has to
    // agree with the fixture.
    const planted: { code: string; finding: boolean }[] = [
      { code: "<p>{label}</p>", finding: false },
      { code: "<p>{formatNumber(count, locale)}</p>", finding: false },
      { code: "<p>{count}</p>", finding: true },
      { code: "<Stat value={String(count)} />", finding: true },
      { code: "<p>{count} / {limit}</p>", finding: true },
      { code: "<p>{maybeCount}</p>", finding: true },
      // Every blind stringifier, by property and by element access.
      { code: "<p>{count.toFixed(2)}</p>", finding: true },
      { code: "<p>{count.toString()}</p>", finding: true },
      { code: '<p>{count["toFixed"](2)}</p>', finding: true },
      { code: "<p>{count.toLocaleString(tag)}</p>", finding: false },
      { code: "<p>{`${count} rows`}</p>", finding: true },
      // Text props, in scope through their declared type.
      { code: "<p title={String(count)}>x</p>", finding: true },
      { code: "<p aria-label={`${count}`}>x</p>", finding: true },
      { code: "<Icon size={count} />", finding: false },
      { code: "<svg viewBox={`0 0 ${count} ${count}`} />", finding: false },
      // Declared on AriaAttributes, so it stays text on an svg.
      { code: "<svg aria-label={String(count)} />", finding: true },
      // A geometry name on one of our own components is still text.
      { code: "<Chart values={String(count)} />", finding: true },
      // A catalog sentence's parameter.
      { code: '<p>{t("k", { n: String(count) })}</p>', finding: true },
      {
        code: '<p>{t("k", { n: formatNumber(count, loc) })}</p>',
        finding: false,
      },
      {
        code: '<p>{t("k", { n: identifierNumber(count) })}</p>',
        finding: false,
      },
      { code: "<Slot body={count} />", finding: true },
      { code: "<Slot body={label} />", finding: false },
      // The rulings themselves.
      { code: "<p>{identifierNumber(count)}</p>", finding: false },
      { code: "<p>{ordinalNumber(count)}</p>", finding: false },
      { code: "<p>{ready ? count : 0}</p>", finding: true },
      { code: "<p>{ready ? label : other}</p>", finding: false },
      // A stray zero is a different defect, fixed by `count > 0 &&`.
      { code: "<p>{count && <b>x</b>}</p>", finding: false },
    ];
    const reported = new Set(
      plant(
        planted.map(({ code }) => code),
        blind,
        geometry,
      ).map(({ line }) => line),
    );
    const verdicts = planted.map(({ code, finding }, index) => ({
      code,
      finding,
      reported: reported.has(index + 1),
    }));
    expect(verdicts.filter((row) => row.finding !== row.reported)).toEqual([]);
  });
});

describe("the jsx-magnitude rule", () => {
  // The rule's own corpus and verdict over a planted tree, typed in this thread.
  const judged = (planted: Parameters<typeof treeOf>[0]) => {
    const tree = treeOf(planted);
    return judge(rule.corpus(tree), tree);
  };
  const screen = (text: string) => ({
    path: join(srcDir, "features", "planted", "plantedcount.tsx"),
    area: "src" as const,
    text: [
      "declare const count: number;",
      "declare const label: string;",
      text,
    ].join("\n"),
  });

  it("reports a raw magnitude drawn in a screen", () => {
    expect(
      judged([screen("export const Row = () => <p>{count}</p>;")]),
    ).toEqual([
      "src/features/planted/plantedcount.tsx:3: a raw magnitude: count",
    ]);
  });

  it("passes text drawn in a screen", () => {
    expect(
      judged([screen("export const Row = () => <p>{label}</p>;")]),
    ).toEqual([]);
  });
});
