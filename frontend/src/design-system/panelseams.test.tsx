/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readFileSync } from "node:fs";
import { dirname } from "node:path";
import { fileURLToPath } from "node:url";
import { cleanup, render } from "@testing-library/react";
import { Fragment, type ReactNode } from "react";
import { afterAll, afterEach, beforeAll, describe, expect, it } from "vitest";
import { selectorList, splitTopLevel } from "../../scripts/lib/css-rules";
import { filesMatching } from "../../scripts/lib/source-tree";
import { withoutComments } from "../testing/css";
import { Disclosure, TableScroll } from "./atoms";
import {
  Panel,
  PanelBody,
  PanelGroupHead,
  PanelIntro,
  PanelRow,
} from "./panel";
import { SettingList, SettingRow } from "./settingrow";

// One line between two bands of a pane, and it runs edge to edge. The shapes
// are the ones settings screens stack, rendered under every design-system sheet.
// happy-dom paints no pseudo-element, so each `::before` rule is copied onto its
// element as custom properties, and happy-dom's cascade picks the winner.
//
// Blind to: screen sheets, `::after`, a shadow or ground drawn as a line, and
// any media query below the 1280px window.

const here = dirname(fileURLToPath(import.meta.url));

const PROBED: Readonly<Record<string, string>> = {
  content: "--seam-content",
  inset: "--seam-inset",
  height: "--seam-height",
  background: "--seam-background",
  "background-color": "--seam-background",
};

function declarations(body: string): [string, string][] {
  return body.split(";").flatMap((declaration) => {
    const colon = declaration.indexOf(":");
    if (colon < 0) return [];
    const property = declaration.slice(0, colon).trim();
    return [[property, declaration.slice(colon + 1).trim()]];
  });
}

const BEFORE = /::?before$/;

// happy-dom keeps `padding-inline` apart from `padding-left`, so a logical
// declaration is rewritten to the physical sides it sets left to right.
const LOGICAL = /^(padding|margin)-inline(-start|-end)?$/;

function physical([property, value]: [string, string]): string[] {
  const logical = LOGICAL.exec(property);
  if (!logical) return [`${property}: ${value}`];
  const [box, side] = [logical[1], logical[2]];
  if (side === "-start") return [`${box}-left: ${value}`];
  if (side === "-end") return [`${box}-right: ${value}`];
  const [start, end = start] = splitTopLevel(value, " ");
  return [`${box}-left: ${start}`, `${box}-right: ${end}`];
}

// Each `X::before` rule also gains a twin on `X` declaring the probed values as
// custom properties, so specificity and order settle them as they settle paint.
function prepared(sheet: string): string {
  return withoutComments(sheet).replace(
    /([^{}]+)\{([^{}]*)\}/g,
    (_, prelude: string, body: string) => {
      const own = declarations(body);
      const rule = `${prelude}{ ${own.flatMap(physical).join("; ")} }`;
      const subjects = selectorList(prelude)
        .map((selector) => selector.trim())
        .filter((selector) => BEFORE.test(selector));
      const probes = own
        .filter(([property]) => property in PROBED)
        .map(([property, value]) => `${PROBED[property]}: ${value}`);
      if (subjects.length === 0 || probes.length === 0) return rule;
      const hosts = subjects.map((s) => s.replace(BEFORE, "") || "*");
      return `${rule}\n${hosts.join(", ")} { ${probes.join("; ")} }`;
    },
  );
}

const sheets = filesMatching(here, /\.css$/);
let installed: HTMLStyleElement | undefined;

beforeAll(() => {
  installed = document.createElement("style");
  installed.textContent = [
    "* { --seam-content: none; --seam-inset: auto; --seam-height: auto; --seam-background: none; }",
    ...sheets.map((sheet) => prepared(readFileSync(sheet, "utf8"))),
  ].join("\n");
  document.head.append(installed);
});
afterAll(() => installed?.remove());
afterEach(cleanup);

// A custom property keeps `var()` unresolved, so it is resolved against :root.
function resolved(value: string): string {
  return value.replace(/var\((--[\w-]+)\)/g, (_, name: string) =>
    resolved(
      getComputedStyle(document.documentElement).getPropertyValue(name).trim(),
    ),
  );
}

// Lengths as the sheets spell them: `0`, `20px`, and `calc(-1 * 20px)`.
function px(value: string): number {
  const text = resolved(value).trim();
  if (text === "" || text === "auto" || text === "0") return 0;
  const length = /^(-?[\d.]+)px$/.exec(text);
  if (length) return Number(length[1]);
  const product = /^calc\((-?[\d.]+)\s*\*\s*(-?[\d.]+)px\)$/.exec(text);
  if (product) return Number(product[1]) * Number(product[2]);
  throw new Error(`no reading for the length "${text}"`);
}

type Line = Readonly<{ owner: Element; left: number; right: number }>;

// How far the element's border box stands in from the pane's two inner edges.
function standoff(element: Element, pane: Element): [number, number] {
  let left = 0;
  let right = 0;
  for (let node = element; node !== pane; ) {
    const style = getComputedStyle(node);
    left += px(style.marginLeft);
    right += px(style.marginRight);
    const parent = node.parentElement;
    if (!parent) throw new Error("the element is not inside the pane");
    if (parent !== pane) {
      const outer = getComputedStyle(parent);
      left += px(outer.paddingLeft) + px(outer.borderLeftWidth);
      right += px(outer.paddingRight) + px(outer.borderRightWidth);
    }
    node = parent;
  }
  return [left, right];
}

function drawn(width: string, style: string): boolean {
  return px(width) > 0 && style !== "none" && style !== "hidden";
}

function borderLine(
  element: Element,
  pane: Element,
  side: "top" | "bottom",
): Line | undefined {
  const style = getComputedStyle(element);
  const width = style.getPropertyValue(`border-${side}-width`);
  if (!drawn(width, style.getPropertyValue(`border-${side}-style`))) return;
  const [left, right] = standoff(element, pane);
  return { owner: element, left, right };
}

function beforeLine(element: Element, pane: Element): Line | undefined {
  const style = getComputedStyle(element);
  const content = style.getPropertyValue("--seam-content").trim();
  const height = style.getPropertyValue("--seam-height").trim();
  const ground = style.getPropertyValue("--seam-background").trim();
  if (content === "none" || height !== "1px" || ground === "none") return;
  const sides = resolved(style.getPropertyValue("--seam-inset")).split(/\s+/);
  const [, insetRight = sides[0], , insetLeft = insetRight] = sides;
  const [left, right] = standoff(element, pane);
  return {
    owner: element,
    left: left + px(insetLeft),
    right: right + px(insetRight),
  };
}

// Down the first (or last) children while only whitespace stands before them.
// Every line on that chain sits in one gap, with no content between.
function chain(band: Element, end: "first" | "last"): Element[] {
  const nodes: Element[] = [];
  for (let node: Element | null = band; node; ) {
    nodes.push(node);
    const children: ChildNode[] = [...node.childNodes].filter(
      (child) => child.nodeType !== 3 || child.textContent?.trim() !== "",
    );
    const edge: ChildNode | undefined =
      end === "first" ? children[0] : children.at(-1);
    node = edge instanceof Element ? edge : null;
  }
  return nodes;
}

// A run of `PanelRow`s is one band; its own rules are the list's, not seams.
function bands(pane: Element): Element[] {
  return [...pane.children].filter(
    (child, at, all) =>
      !(child.matches(".panel-row") && all[at - 1]?.matches(".panel-row")),
  );
}

function seamLines(pane: Element, above: Element, below: Element): Line[] {
  const before = above.matches(".panel-row")
    ? (lastOfRun(above) ?? above)
    : above;
  return [
    ...chain(before, "last").map((node) => borderLine(node, pane, "bottom")),
    ...chain(below, "first").flatMap((node) => [
      borderLine(node, pane, "top"),
      beforeLine(node, pane),
    ]),
  ].filter((line): line is Line => line !== undefined);
}

function lastOfRun(row: Element): Element | null {
  let last = row;
  while (last.nextElementSibling?.matches(".panel-row")) {
    last = last.nextElementSibling;
  }
  return last;
}

function paneOf(container: HTMLElement): Element {
  const pane = container.querySelector(".panel");
  if (!pane) throw new Error("the shape rendered no panel");
  return pane;
}

// Lines at each boundary, from the head down.
function seams(pane: Element): Line[][] {
  const stack = bands(pane);
  return stack.slice(1).map((band, at) => seamLines(pane, stack[at], band));
}

// Every line in the pane but the rule between two plain rows: that is a list's
// rule, and it stops at the gutter.
function everyLine(pane: Element): Line[] {
  return [...pane.querySelectorAll("*")]
    .flatMap((node) => [
      borderLine(node, pane, "top"),
      borderLine(node, pane, "bottom"),
      node.matches(".panel-row + .panel-row:not(.panel-row-record)")
        ? undefined
        : beforeLine(node, pane),
    ])
    .filter((line): line is Line => line !== undefined);
}

function describeLine(line: Line): string {
  return `${line.owner.className || line.owner.tagName} ${line.left}/${line.right}`;
}

const rows = (
  <SettingList bleed="settings">
    <SettingRow label="Sync" control={<span>On</span>} />
    <SettingRow label="Window" control={<span>30 days</span>} />
  </SettingList>
);

const table = (
  <TableScroll label="Spend" bleed>
    <table className="table">
      <thead>
        <tr>
          <th>Task</th>
          <th>Calls</th>
        </tr>
      </thead>
      <tbody>
        <tr>
          <td>Triage</td>
          <td>12</td>
        </tr>
        <tr>
          <td>Drafts</td>
          <td>4</td>
        </tr>
      </tbody>
    </table>
  </TableScroll>
);

const fold = (
  <Disclosure summary="Show days" open>
    <p>Twelve calls on Monday.</p>
  </Disclosure>
);

// Lines at each boundary, head first. 0 is a boundary the pane leaves open.
const SHAPES: readonly (readonly [string, ReactNode, readonly number[]])[] = [
  [
    "body, body",
    [<PanelBody key="a">One.</PanelBody>, <PanelBody key="b">Two.</PanelBody>],
    [1, 1],
  ],
  [
    "body, body opening with a disclosure",
    [
      <PanelBody key="a">One.</PanelBody>,
      <PanelBody key="b">{fold}</PanelBody>,
    ],
    [1, 1],
  ],
  [
    "a disclosure opening the first body",
    <PanelBody key="b">{fold}</PanelBody>,
    [1],
  ],
  [
    "body, a disclosure in the pane",
    [
      <PanelBody key="a">One.</PanelBody>,
      <Disclosure key="b" summary="Older">
        x
      </Disclosure>,
    ],
    [1, 1],
  ],
  [
    "bleed list, body",
    [<Fragment key="a">{rows}</Fragment>, <PanelBody key="b">Two.</PanelBody>],
    [1, 1],
  ],
  [
    "body, bleed list",
    [<PanelBody key="a">One.</PanelBody>, <Fragment key="b">{rows}</Fragment>],
    [1, 0],
  ],
  [
    "bleed table, body",
    [<Fragment key="a">{table}</Fragment>, <PanelBody key="b">Two.</PanelBody>],
    [1, 1],
  ],
  [
    "rows, body",
    [
      <PanelRow key="a">One</PanelRow>,
      <PanelRow key="b">Two</PanelRow>,
      <PanelBody key="c">Three.</PanelBody>,
    ],
    [1, 1],
  ],
  [
    "body, rows",
    [
      <PanelBody key="a">One.</PanelBody>,
      <PanelRow key="b">Two</PanelRow>,
      <PanelRow key="c">Three</PanelRow>,
    ],
    [1, 1],
  ],
  [
    "body, group head, body",
    [
      <PanelBody key="a">One.</PanelBody>,
      <PanelGroupHead key="b" title="By task" level="h3" />,
      <PanelBody key="c">Two.</PanelBody>,
    ],
    [1, 1, 0],
  ],
  [
    "bleed list, group head, bleed list",
    [
      <Fragment key="a">{rows}</Fragment>,
      <PanelGroupHead key="b" title="Senders" level="h3" />,
      <Fragment key="c">{rows}</Fragment>,
    ],
    [1, 1, 0],
  ],
];

function renderShape(children: ReactNode): Element {
  const { container } = render(<Panel title="Usage">{children}</Panel>);
  return paneOf(container);
}

describe("a pane draws one line between two bands, edge to edge", () => {
  it("reads every design-system sheet and copies the seam rules it draws", () => {
    expect(sheets.length).toBeGreaterThan(40);
    const probed = prepared(readFileSync(`${here}/panel.css`, "utf8"));
    expect(probed.match(/--seam-content/g)?.length ?? 0).toBeGreaterThan(3);
  });

  it.each(SHAPES)("%s", (_, children, expected) => {
    const pane = renderShape(children);
    const found = seams(pane);
    expect(found.map((lines) => lines.length)).toEqual(expected);
    const short = found
      .flat()
      .filter((line) => line.left !== 0 || line.right !== 0);
    expect(short.map(describeLine)).toEqual([]);
  });

  it("sees a second line at a boundary when one is drawn", () => {
    const pane = renderShape([
      <PanelBody key="a">One.</PanelBody>,
      <PanelBody key="b">
        <hr style={{ border: 0, borderTop: "1px solid", margin: 0 }} />
      </PanelBody>,
    ]);
    expect(seams(pane).map((lines) => lines.length)).toEqual([1, 2]);
  });
});

describe("every line in a pane shares the pane's width", () => {
  it.each([
    [
      "a list nested in a disclosure of a bleed list",
      <SettingList key="a" bleed="settings">
        <SettingRow label="Name" control={<span>Acme</span>} />
        <Disclosure summary="Market" open>
          <SettingList>
            <SettingRow label="Sells" control={<span>Pumps</span>} />
            <SettingRow label="Buyers" control={<span>Plants</span>} />
          </SettingList>
        </Disclosure>
      </SettingList>,
    ],
    [
      "a disclosure under an intro in one body",
      <PanelBody key="a">
        <PanelIntro>What each task cost this month.</PanelIntro>
        {fold}
      </PanelBody>,
    ],
    [
      "a list nested in a disclosure of a body",
      [
        <PanelBody key="a">One.</PanelBody>,
        <PanelBody key="b">
          <Disclosure summary="Audit" open>
            <SettingList>
              <SettingRow label="Renamed" control={<span>Today</span>} />
              <SettingRow label="Added" control={<span>Monday</span>} />
            </SettingList>
          </Disclosure>
        </PanelBody>,
      ],
    ],
  ])("%s", (_, children) => {
    const pane = renderShape(children);
    const lines = everyLine(pane);
    expect(lines.length).toBeGreaterThan(1);
    const short = lines.filter((line) => line.left !== 0 || line.right !== 0);
    expect(short.map(describeLine)).toEqual([]);
  });
});
