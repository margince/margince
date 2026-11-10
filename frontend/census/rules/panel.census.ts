// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// panel.css owns the pane: the head band's height, edge and title, and the
// inset of a state arm the pane has no body to wrap. base.css owns the
// document's hyphenation limit. No other sheet in the product sets these.
//
// The rules read declarations, not a rendered box, because jsdom lays nothing
// out. A screen re-spacing the band in a sheet of its own is the defect.

import { join, relative } from "node:path";
import { selectorList, subjectsOf } from "../../scripts/lib/css-rules";
import { type CensusFile, type CensusRule, srcDir } from "../tree";

const designSystem = join(srcDir, "design-system");
export const PANEL_SHEET = join(designSystem, "panel.css");
const BASE_SHEET = join(designSystem, "base.css");

// One rule as the sweeps read it: what it styles, and what it declares.
export type CssRule = Readonly<{
  selector: string;
  block: string;
  properties: readonly string[];
}>;

// The band's own geometry: how tall it is, the air inside it, how its content
// lines up. Whoever sets one decides the height of every panel head.
export const BAND_GEOMETRY =
  /^(?:height|min-height|max-height|padding(?:-top|-bottom|-block|-block-start|-block-end)?|align-items|flex-wrap)$/;
// The band's lower edge is the card's own chrome. A tone recolours it; no
// other sheet redraws or drops it.
const BAND_EDGE = /^border-bottom(?:-|$)/;
// How the title is set. `font` is here because the shorthand carries the size.
const TITLE_TYPE =
  /^(?:font|font-size|font-weight|font-family|line-height|letter-spacing)$/;

export function stripComments(css: string): string {
  return css.replace(/\/\*[\s\S]*?\*\//g, "");
}

// A block at-rule wraps ordinary rules, so dropping its prelude and brace
// leaves them at the top level. A statement at-rule (@import) is dropped to
// its semicolon only, so the selector after it survives.
function unwrapAtRules(css: string): string {
  return css.replace(/@[a-zA-Z-]+[^{};]*[{;]/g, "");
}

/** One declaration's value, read by property name. */
export function declaredValue(
  block: string,
  property: string,
): string | undefined {
  for (const declaration of block.split(";")) {
    const colon = declaration.indexOf(":");
    if (colon < 0) continue;
    if (declaration.slice(0, colon).trim().toLowerCase() === property) {
      return declaration.slice(colon + 1).trim();
    }
  }
  return undefined;
}

function declaredProperties(block: string): readonly string[] {
  return block
    .split(";")
    .map((declaration) => declaration.split(":")[0].trim().toLowerCase())
    .filter((property) => /^[a-z-]+$/.test(property));
}

export function cssRules(css: string): readonly CssRule[] {
  const flat = unwrapAtRules(stripComments(css));
  return [...flat.matchAll(/([^{}]+)\{([^{}]*)\}/g)].flatMap(
    ([, selectors, block]) =>
      selectorList(selectors).map((selector) => ({
        // One space per combinator, whatever the sheet wrapped across lines.
        selector: selector.replace(/\s+/g, " "),
        block,
        properties: declaredProperties(block),
      })),
  );
}

// A rule styles the last compound of its selector: `.pe-memory .panel-head`
// re-shapes the band, and `.panel-head-count` is content inside it.
function stylesTheBand(selector: string): boolean {
  return subjectsOf(selector).some((subject) =>
    /^\.panel-head(?![\w-])/.test(subject),
  );
}

export function bandRules(css: string): readonly CssRule[] {
  return cssRules(css).filter((rule) => stylesTheBand(rule.selector));
}

// The title, wherever it hangs. The dot matters: `.rmap-panel-title` is the
// relationship map's own aside.
function stylesTheTitle(selector: string): boolean {
  return subjectsOf(selector).some((subject) =>
    /(?:^|[^\w-])\.panel-title(?![\w-])/.test(subject),
  );
}

export function titleRules(css: string): readonly CssRule[] {
  return cssRules(css).filter((rule) => stylesTheTitle(rule.selector));
}

// The rules that decide how a title is set. Recolouring is not one of them.
export function titleTypeRules(css: string): readonly CssRule[] {
  return titleRules(css).filter((rule) =>
    rule.properties.some((property) => TITLE_TYPE.test(property)),
  );
}

// A state arm is the whole thing `PendingBody` or `EmptyState` draws.
// `.pending-line` and `.empty-plate` are parts of one.
function isStateArm(selector: string): boolean {
  return subjectsOf(selector).some((subject) =>
    /^\.(?:pending|empty)$/.test(subject),
  );
}

const INSET = /^padding(?:-inline(?:-start|-end)?|-left|-right)?$/;

// A pane's inset spelled as the role. On a state arm it claims the surface
// holding it is a pane, which only the pane's own sheet may claim.
const PANE_INSET = /var\(--pad(?:Panel|Card)\)/;

// An inset a container decides for the arm in it, which a combinator in the
// selector says. `.empty` alone is `EmptyState` sizing itself.
export function stateArmRoleInsets(css: string): readonly CssRule[] {
  return cssRules(css).filter(
    (rule) =>
      isStateArm(rule.selector) &&
      /[ >+~]/.test(rule.selector) &&
      rule.properties.some((property) => INSET.test(property)) &&
      PANE_INSET.test(rule.block),
  );
}

/** Every finding one sheet makes against panel.css or base.css. */
export function sheetFindings(sheet: CensusFile): string[] {
  const where = relative(srcDir, sheet.path);
  const said = (rule: CssRule, property: string) =>
    `${where}: ${rule.selector} sets ${property}`;
  const owned =
    sheet.path === PANEL_SHEET
      ? []
      : [
          ...bandRules(sheet.text).flatMap((rule) =>
            rule.properties
              .filter(
                (property) =>
                  BAND_GEOMETRY.test(property) || BAND_EDGE.test(property),
              )
              .map((property) => said(rule, property)),
          ),
          ...titleRules(sheet.text).flatMap((rule) =>
            rule.properties
              .filter((property) => TITLE_TYPE.test(property))
              .map((property) => said(rule, property)),
          ),
          ...stateArmRoleInsets(sheet.text).map(
            (rule) =>
              `${where}: ${rule.selector} spells a pane's inset on a state arm`,
          ),
        ];
  const hyphenates =
    sheet.path !== BASE_SHEET &&
    /hyphenate-limit-chars\s*:/.test(stripComments(sheet.text));
  return hyphenates
    ? [...owned, `${where}: sets hyphenate-limit-chars`]
    : owned;
}

const isSheet = (file: CensusFile) =>
  (file.area === "src" || file.area === "extensions") &&
  file.path.endsWith(".css");

// Sheets that carried a copy once, by name: a walk that missed them would
// sweep past and report a pass.
const NAMED = [
  "features/deals/deal360/deal360.css",
  "features/contacts/contact360.css",
  "features/automations/automationdetail.css",
  "design-system/settingrow.css",
  "design-system/base.css",
  "design-system/panel.css",
];

export const rule: CensusRule = {
  name: "panel",
  corpus: (tree) => tree.files.filter(isSheet),
  floors: (corpus) => {
    const read = new Set(corpus.map((file) => relative(srcDir, file.path)));
    return NAMED.filter((name) => !read.has(name)).map(
      (name) => `the sweep did not reach src/${name}`,
    );
  },
  visit: sheetFindings,
  advice:
    "the head band's floor is 56px and its title one size on every screen: recolour them if you must, resize them in panel.css or not at all. The pane insets its own un-wrappable state arm, and base.css sets the hyphenation limit once.",
};
