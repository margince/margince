// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { stable } from "../../src/format/collate";
import { plantedFindings, srcDir, treeOf } from "../tree";
import {
  APP,
  APP_SCREENS,
  APP_VIEWS,
  bodyOfComponent,
  insetVerdict,
  NAV,
  ROUTER,
  raillessComponents,
  rootsOfBody,
  rule,
  walkOver,
} from "./page-gutter-coverage.census";

describe("the page-gutter-coverage census", () => {
  it.each([
    [
      "both branches inset",
      '<Suspense fallback={<div className="wrap" />}><div className="wrap" /></Suspense>',
      true,
    ],
    [
      "text fallback",
      '<Suspense fallback="Loading"><div className="wrap" /></Suspense>',
      false,
    ],
    [
      "spread fallback",
      '<Suspense {...props}><div className="wrap" /></Suspense>',
      false,
    ],
    [
      "flush fallback",
      '<Suspense fallback={<div />}><div className="wrap" /></Suspense>',
      false,
    ],
    [
      "flush loaded view",
      '<Suspense fallback={<div className="wrap" />}><div /></Suspense>',
      false,
    ],
    [
      "aliased boundary",
      '<Wait fallback={<div className="wrap" />}><div className="wrap" /></Wait>',
      true,
    ],
    [
      "namespace boundary",
      '<React.Suspense fallback={<div className="wrap" />}><div className="wrap" /></React.Suspense>',
      true,
    ],
  ])("checks every Suspense branch: %s", (name, expression, expected) => {
    const fixture = parseSource(
      `${name}.tsx`,
      `import { Suspense, Suspense as Wait } from "react"; import * as React from "react"; function Screen() { return ${expression}; }`,
    );
    const body = bodyOfComponent(fixture, "Screen");
    if (!body) throw new Error("fixture has no screen body");
    const walk = walkOver(treeOf([]), new Set());
    expect(
      insetVerdict(fixture, rootsOfBody(body), walk, new Set()).inset,
    ).toBe(expected);
  });

  it("finds railless screens below a provider without exempting HTML tags", () => {
    const source = parseSource(
      "App.tsx",
      `
      const view = <RaillessFrame><CompleteLocale><ResetRoute />
        <div><AuthScreen /></div>
      </CompleteLocale></RaillessFrame>;
    `,
    );
    expect([...raillessComponents(source)].sort(stable)).toEqual([
      "AuthScreen",
      "CompleteLocale",
      "ResetRoute",
    ]);
  });
});

describe("the page-gutter-coverage rule", () => {
  const shell = (screen: string) => [
    {
      path: ROUTER,
      text: 'export const SCREENS = ["home", "inset", "flat"] as const;',
    },
    { path: NAV, text: 'export const RAIL_LESS_SCREENS = new Set(["home"]);' },
    {
      path: APP,
      text: "const v = <RaillessFrame><AuthScreen /></RaillessFrame>;",
    },
    {
      path: APP_VIEWS,
      text: "const SCREEN_VIEWS = { home: () => <Home />, inset: () => <Inset />, flat: () => <Flat /> };",
    },
    {
      path: APP_SCREENS,
      text: 'import { Inset } from "./features/inset";\nimport { Flat } from "./features/flat";\n',
    },
    {
      path: join(srcDir, "features", "inset.tsx"),
      text: 'export function Inset() { return <div className="wrap" />; }',
    },
    { path: join(srcDir, "features", "flat.tsx"), text: screen },
  ];
  const planted = (screen: string) =>
    plantedFindings(
      rule,
      shell(screen).map((file) => ({ ...file, area: "src" as const })),
    );

  it("reports a planted screen that renders flush", () => {
    expect(
      planted('export function Flat() { return <div className="body" />; }'),
    ).toEqual([
      'flat (src/App.screens.tsx): Flat → root <div class="body"> takes no page gutter',
    ]);
  });

  it("passes a screen that insets itself in its own stylesheet", () => {
    const files = [
      ...shell('export function Flat() { return <div className="pane" />; }'),
      {
        path: join(srcDir, "features", "flat.css"),
        text: ".pane { padding: 0 var(--pageGutter); }",
      },
    ].map((file) => ({ ...file, area: "src" as const }));
    expect(plantedFindings(rule, files)).toEqual([]);
  });
});
