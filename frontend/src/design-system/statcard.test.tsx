/** @vitest-environment happy-dom */

import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { cleanup, render, screen } from "@testing-library/react";
import ts from "typescript";
import { afterEach, describe, expect, it } from "vitest";
import {
  extensionFrontendFiles,
  filesMatching,
  sourceFileAt,
} from "../../scripts/lib/source-tree";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { StatCard } from "./statcard";

// THE DOOR OUT OF A READING SAYS "Open", AND NOTHING ELSE EVER.
//
// It used to be the caller's word: `openLabel` let each reading name its own
// destination, and what the product got was several spellings of one control —
// "Review urgent work", "View today's meetings", "Review leads owed a reply" —
// which is a different door on every card to anybody reading them as a set.
// The reading is already named on the card the door sits in, and a screen
// reader hears it as the button's DESCRIPTION, so the word itself has one job
// and one spelling.
//
// Three assertions, because the prop can come back three ways and each is
// invisible to the other two: as a rendered word (the render test), as an
// attribute somewhere in the tree (the sweep), and as a second word written
// straight into the foot (the source check). The fourth holds the message the
// first one reads, so a door that says "Öffnen" in `en` still fails here.

const here = dirname(fileURLToPath(import.meta.url));
const sourceRoot = join(here, "..");
// A unit's screen is shipped UI in the same bundle, so a gate stopping at
// frontend/src would hold the core to a rule the extension tier escapes.
const extensionsRoot = join(here, "..", "..", "..", "extensions");

afterEach(cleanup);

function drawReading() {
  return render(
    <LocaleProvider initial="en">
      <StatCard
        label="Urgent"
        value="4"
        detail="somebody waiting"
        onOpen={() => {}}
      />
    </LocaleProvider>,
  );
}

describe("the door always says Open", () => {
  it("letters the foot's button with the stat.open message and no other word", () => {
    const { container } = drawReading();

    const door = screen.getByRole("button");
    expect(door.textContent?.replace(/→/g, "").trim()).toBe(en["stat.open"]);
    // The foot holds that button and nothing beside it: a second word here is
    // the caller's label arriving under another name.
    const foot = container.querySelector(".stat-card-foot");
    expect(foot?.textContent?.replace(/→/g, "").trim()).toBe(en["stat.open"]);
  });

  it("names the ACTION and describes the reading, never both in the name", () => {
    const { container } = drawReading();

    const door = screen.getByRole("button");
    const described = (door.getAttribute("aria-describedby") ?? "")
      .split(/\s+/)
      .map((id) => container.ownerDocument.getElementById(id)?.textContent)
      .join(" ");
    expect(described).toContain("Urgent");
    expect(door.getAttribute("aria-label")).toBeNull();
  });

  it("pins the English word, which is what every door in the product reads", () => {
    expect(en["stat.open"]).toBe("Open");
  });
});

// The corpus is every screen and primitive that could carry the prop back.
// Stories and tests are excluded because they are allowed to talk ABOUT it —
// this file does, above — and a sweep that read them would fail on its own
// prose. Floored, because a walk that silently reads a smaller tree reports the
// same word this one does when it is clean.
function appTsx(): string[] {
  return filesMatching(sourceRoot, /\.tsx$/)
    .concat(extensionFrontendFiles(extensionsRoot))
    .filter((path) => !/\.(stories|test)\.tsx$/.test(path));
}

describe("no reading names its own door", () => {
  // Parses every application .tsx, core and extension, for one attribute name.
  it("finds no openLabel attribute anywhere the app is written", {
    timeout: 60_000,
  }, () => {
    const files = appTsx();
    // The tree carried six hundred of these when this was written; a corpus
    // that has fallen to a fraction of that is a miswired walk reporting the
    // same word a clean tree does.
    expect(files.length).toBeGreaterThan(400);
    // And the extension tier specifically: the floor above is one the core
    // satisfies alone, so it cannot notice a walk that stops at src/.
    expect(
      files.some((file) => file.includes("/extensions/")),
      "the census reached no extension frontend layer",
    ).toBe(true);

    const offenders = files.flatMap((path) => {
      const source = sourceFileAt(path);
      const hits: string[] = [];
      const visit = (node: ts.Node): void => {
        if (
          ts.isJsxAttribute(node) &&
          ts.isIdentifier(node.name) &&
          node.name.text === "openLabel"
        ) {
          const { line } = source.getLineAndCharacterOfPosition(
            node.getStart(),
          );
          hits.push(`${relative(sourceRoot, path)}:${line + 1}`);
        }
        ts.forEachChild(node, visit);
      };
      visit(source);
      return hits;
    });

    expect(offenders).toEqual([]);
  });
});

describe("the foot speaks once", () => {
  it("writes exactly one message into the card's foot", () => {
    const path = join(here, "statcard.tsx");
    const source = sourceFileAt(path);

    const foot = findElementByClass(source, "stat-card-foot");
    expect(foot).not.toBeUndefined();
    if (foot === undefined) return;

    // Every word the foot can draw: a `t(...)` lookup or a literal written
    // straight in. One lookup, no literals — a second of either is a door
    // saying something the rest of the product's doors do not.
    expect(messageLookups(foot)).toEqual(['t("stat.open")']);
    expect(literalWords(foot)).toEqual([]);
  });
});

/** The JSX element whose className attribute is exactly `name`. */
function findElementByClass(
  source: ts.SourceFile,
  name: string,
): ts.Node | undefined {
  let found: ts.Node | undefined;
  const visit = (node: ts.Node): void => {
    if (found !== undefined) return;
    const opening = ts.isJsxElement(node)
      ? node.openingElement
      : ts.isJsxSelfClosingElement(node)
        ? node
        : undefined;
    if (opening !== undefined && classNameOf(opening) === name) {
      found = node;
      return;
    }
    ts.forEachChild(node, visit);
  };
  visit(source);
  return found;
}

function classNameOf(
  opening: ts.JsxOpeningElement | ts.JsxSelfClosingElement,
): string | undefined {
  for (const attribute of opening.attributes.properties) {
    if (
      ts.isJsxAttribute(attribute) &&
      ts.isIdentifier(attribute.name) &&
      attribute.name.text === "className" &&
      attribute.initializer !== undefined &&
      ts.isStringLiteral(attribute.initializer)
    ) {
      return attribute.initializer.text;
    }
  }
  return undefined;
}

/** Every `t("…")` call written inside `root`, as source text. */
function messageLookups(root: ts.Node): string[] {
  const calls: string[] = [];
  const visit = (node: ts.Node): void => {
    if (
      ts.isCallExpression(node) &&
      ts.isIdentifier(node.expression) &&
      node.expression.text === "t"
    ) {
      calls.push(node.getText());
    }
    ts.forEachChild(node, visit);
  };
  visit(root);
  return calls;
}

/** Every word written into `root` as text rather than looked up. */
function literalWords(root: ts.Node): string[] {
  const words: string[] = [];
  const visit = (node: ts.Node): void => {
    if (ts.isJsxText(node) && node.text.trim() !== "") {
      words.push(node.text.trim());
    }
    // Only a string the JSX DRAWS counts: one inside a `{…}` child. A class
    // name, a DOM flag and the key handed to `t()` are all strings too, and
    // none of them is a word the reader meets. Nor is the arrow — it is
    // `aria-hidden` on its own span, so a screen reader never reaches it.
    if (
      ts.isStringLiteral(node) &&
      node.parent !== undefined &&
      ts.isJsxExpression(node.parent) &&
      node.text.trim() !== "" &&
      node.text.trim() !== "→"
    ) {
      words.push(node.text.trim());
    }
    ts.forEachChild(node, visit);
  };
  visit(root);
  return words;
}
