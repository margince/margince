// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";
import { describe, expect, it } from "vitest";
import {
  filesMatching,
  parseSource,
  sourceFileAt,
} from "../../scripts/lib/source-tree";

// A titled boxed surface standing in a page's column is a `Panel`, whatever
// the page. A `Card` wearing a `title` is the same claim drawn by the other
// primitive, and the two do not agree: the panel's head is one fixed band at
// one height with the title at one size, which is what lets a reader scan a
// column of zones by their titles alone. A card's header grows with its
// description and sits on the card's own padding, so a page mixing the two
// draws the same zone at three heights and reads as two products stitched
// together — the split this tree already made once between settings and the
// record page. It was read as a rule about RECORD pages once, which left the
// dashboards and the builder screens drawing the other card.
//
// Two shapes of the offence, because the tree grew both. The prop form,
// `<Card title=…>`, renders the card's own `SectionHeader`. The hand-rolled
// form puts that `SectionHeader` in by hand under a padding wrapper — six of
// them stood in one screen file — and a gate reading only the prop would call
// that file clean.
//
// WHAT IT DELIBERATELY DOES NOT CATCH: an UNTITLED `Card`. A list item, an
// inset nested inside a panel, an auth card, a kanban card, a dialog body — a
// card is still the one card surface, and most of them are not zones. Only the
// head band is the zone claim, so only the head band is asked about; firing on
// every card would be this gate asserting an answer nobody gave, and a gate
// that fires on correct code teaches readers to skip its output.

const frontendRoot = join(dirname(fileURLToPath(import.meta.url)), "..", "..");
const sourceRoot = join(frontendRoot, "src");
const screensRoot = join(sourceRoot, "screens");

/**
 * The screens whose titled `Card` stands somewhere other than a page column,
 * each with the reason — the product owner's call per file, written where the
 * gate reads it rather than in a document the next author will not open.
 *
 * Every one of them is a card INSIDE something: a row of a list, a slot of a
 * drawing, a row of a timeline. A zone claims a page's column; these claim a
 * place in a surface that already has a head of its own.
 *
 * A file, not a line: a line number moves whenever anything above it is edited,
 * and a baseline that churns on unrelated changes is one people regenerate
 * without reading.
 */
const notARecordZone = new Map<string, string>([
  [
    "screens/brief.weekly.learnings.tsx",
    "one learning of a list, titled by kind, inside the learnings panel's body",
  ],
  [
    "screens/personnetwork/edgedetail.tsx",
    "the selected node's detail in the relationship map's own panel slot, level 3 under the map's heading",
  ],
  [
    "screens/transcriptread.tsx",
    "an inset inside one activity-timeline row, which is where the timeline mounts it",
  ],
]);

/** The head band a card draws when it is claiming to be a zone. */
const HEAD_BAND = "SectionHeader";

/**
 * The tags that own a head of their own. A `SectionHeader` inside one of these
 * is that surface's head, not the head of the card around it, so the walk stops
 * at the boundary rather than reading a nested card's title as its parent's.
 */
const OWNS_ITS_HEAD = new Set(["Card", "Panel", "RailPanel", "Modal"]);

/** Every screen module, minus the two kinds of file that are not the screen. */
function screenModules(): string[] {
  return filesMatching(screensRoot, /\.tsx$/).filter(
    (path) => !/\.(test|stories)\.tsx$/.test(path),
  );
}

/** A path as the allowlist spells it: relative to `src/`, forward slashes. */
function pathOf(file: string): string {
  return relative(sourceRoot, file).replaceAll("\\", "/");
}

/**
 * The JSX children a reader actually sees. Whitespace between elements is
 * dropped, and an expression wrapper is looked THROUGH: `{staged && <SectionHeader/>}`
 * and `{rows.map(() => <SectionHeader/>)}` both render the header, and a walk
 * that stopped at the `{` would read the card as untitled.
 */
function renderedChildren(children: readonly ts.JsxChild[]): ts.JsxChild[] {
  return children.flatMap((child) => {
    if (ts.isJsxText(child)) {
      return child.containsOnlyTriviaWhiteSpaces ? [] : [child];
    }
    if (ts.isJsxFragment(child)) {
      return renderedChildren(child.children);
    }
    if (ts.isJsxExpression(child)) {
      return child.expression === undefined ? [] : jsxWithin(child.expression);
    }
    return [child];
  });
}

/** The JSX an expression renders, however many guards and calls are in the way. */
function jsxWithin(node: ts.Node): ts.JsxChild[] {
  if (
    ts.isJsxElement(node) ||
    ts.isJsxSelfClosingElement(node) ||
    ts.isJsxFragment(node)
  ) {
    return [node];
  }
  const found: ts.JsxChild[] = [];
  ts.forEachChild(node, (child) => {
    found.push(...jsxWithin(child));
  });
  return found;
}

function tagOf(child: ts.JsxChild): string | undefined {
  if (ts.isJsxElement(child)) {
    return child.openingElement.tagName.getText();
  }
  if (ts.isJsxSelfClosingElement(child)) {
    return child.tagName.getText();
  }
  return undefined;
}

function childrenOf(child: ts.JsxChild): ts.JsxChild[] {
  return ts.isJsxElement(child) ? renderedChildren(child.children) : [];
}

/**
 * The hand-rolled head: a `SectionHeader` among the card's own children, or
 * inside the padding wrapper the card's first child is. Two levels and no
 * more — deeper than that the header belongs to something inside the card
 * rather than to the card itself.
 */
function drawsAHeadBand(card: ts.JsxElement): boolean {
  const children = renderedChildren(card.children);
  if (children.some((child) => tagOf(child) === HEAD_BAND)) {
    return true;
  }
  const first = children[0];
  if (first === undefined || OWNS_ITS_HEAD.has(tagOf(first) ?? "")) {
    return false;
  }
  return childrenOf(first).some((child) => tagOf(child) === HEAD_BAND);
}

function carriesATitle(attributes: ts.JsxAttributes): boolean {
  return attributes.properties.some(
    (property) =>
      ts.isJsxAttribute(property) && property.name.getText() === "title",
  );
}

type TitledCard = { where: string; line: number };

/** Every titled `Card` in one parsed module, in source order. */
function titledCardsIn(where: string, source: ts.SourceFile): TitledCard[] {
  const found: TitledCard[] = [];
  const visit = (node: ts.Node) => {
    const opening = ts.isJsxElement(node)
      ? node.openingElement
      : ts.isJsxSelfClosingElement(node)
        ? node
        : undefined;
    if (opening !== undefined && opening.tagName.getText() === "Card") {
      const titled =
        carriesATitle(opening.attributes) ||
        (ts.isJsxElement(node) && drawsAHeadBand(node));
      if (titled) {
        found.push({
          where,
          line:
            source.getLineAndCharacterOfPosition(opening.getStart()).line + 1,
        });
      }
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

function titledCards(): TitledCard[] {
  return screenModules().flatMap((file) =>
    titledCardsIn(pathOf(file), sourceFileAt(file)),
  );
}

describe("a titled zone is a Panel", () => {
  const modules = screenModules();
  const found = titledCards();

  it("reads a corpus that is the screens tree", () => {
    // A census that reads a smaller tree reports the same word, PASS, and
    // nothing asserts. So the floor is named, and so is a file the walk must
    // have reached — including one a directory deep, because a walk that
    // stopped at the top level clears any floor the flat screens supply.
    expect(modules.length).toBeGreaterThan(80);
    expect(modules.map(pathOf)).toContain("screens/person360.tsx");
    expect(modules.map(pathOf)).toContain(
      "screens/personnetwork/edgedetail.tsx",
    );
  });

  it("finds no titled Card standing in a page column", () => {
    const findings = found
      .filter((card) => !notARecordZone.has(card.where))
      .map(
        (card) =>
          `${card.where}:${card.line} <Card title=…> is a titled zone — use Panel (design-system/panel.tsx); if this card stands inside another surface rather than in a page column, allowlist the file here with the reason`,
      );
    expect(findings, findings.join("\n")).toEqual([]);
  });

  it("keeps no allowlist entry for a file that has none left", () => {
    const carriesOne = new Set(found.map((card) => card.where));
    const stale = [...notARecordZone.keys()]
      .filter((where) => !carriesOne.has(where))
      .map((where) => `${where}: stale waiver: remove the entry`);
    expect(stale, stale.join("\n")).toEqual([]);
  });

  // The detector must be able to SEE each shape, or a clean tree and a blind
  // reader look identical.
  describe("the detector", () => {
    const read = (source: string): TitledCard[] =>
      titledCardsIn("fixture.tsx", parseSource("fixture.tsx", source));

    it("reads the prop form, open and self-closing", () => {
      expect(
        read('<Card title={t("consent.title")}>{rows}</Card>'),
      ).toHaveLength(1);
      expect(read("<Card title={title} sub={sub} />")).toHaveLength(1);
    });

    it("reads the hand-rolled head, direct and under a padding wrapper", () => {
      expect(read("<Card><SectionHeader title={title} /></Card>")).toHaveLength(
        1,
      );
      expect(
        read(
          '<Card>\n  <div style={{ padding: "var(--padCard)" }}>\n    <SectionHeader title={title} />\n  </div>\n  {rows}\n</Card>',
        ),
      ).toHaveLength(1);
    });

    it("reads a head drawn under a condition", () => {
      expect(
        read("<Card>{visible && <SectionHeader title={title} />}</Card>"),
      ).toHaveLength(1);
    });

    it("leaves an untitled Card alone", () => {
      expect(read('<Card as="li" className="grant-row">{name}</Card>')).toEqual(
        [],
      );
      expect(read("<Card inset>{body}</Card>")).toEqual([]);
    });

    it("does not read a Panel's title as a Card's", () => {
      expect(read("<Panel title={title}>{body}</Panel>")).toEqual([]);
    });

    it("credits a nested surface's head to the nested surface", () => {
      const nested = read(
        "<Card>\n  <Card>\n    <SectionHeader title={title} />\n  </Card>\n</Card>",
      );
      expect(nested).toHaveLength(1);
      expect(nested[0].line).toBe(2);
    });

    it("names the line the card opens on", () => {
      expect(read("<div>\n  <Card title={title} />\n</div>")[0].line).toBe(2);
    });
  });
});
