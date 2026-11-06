/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
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
import { Button } from "./atoms";
import { QuietVerbs } from "./verbvolume";

const here = dirname(fileURLToPath(import.meta.url));

afterEach(cleanup);

function face(name: string): string[] {
  return screen.getByRole("button", { name }).className.split(" ");
}

function marked(name: string): boolean {
  return (
    screen.getByRole("button", { name }).querySelector(".lucide-sparkles") !==
    null
  );
}

describe("a control's volume follows its region", () => {
  it("fills primary and ai outside a quiet region", () => {
    render(
      <>
        <Button variant="primary">Save</Button>
        <Button variant="ai">Draft reply</Button>
      </>,
    );
    expect(face("Save")).toContain("btn-primary");
    expect(face("Draft reply")).toContain("btn-ai");
  });

  it("draws both outlined inside one, and keeps danger red", () => {
    render(
      <QuietVerbs>
        <Button variant="primary">Mark done</Button>
        <Button variant="ai">Draft reply</Button>
        <Button variant="danger">Delete</Button>
      </QuietVerbs>,
    );
    expect(face("Mark done")).toContain("btn-ghost");
    expect(face("Draft reply")).toEqual(
      expect.arrayContaining(["btn-ghost", "btn-aiQuiet"]),
    );
    expect(face("Delete")).toContain("btn-danger");
  });

  it("keeps the fill where the region says it is the card in hand", () => {
    render(
      <QuietVerbs quiet={false}>
        <Button variant="ai">Draft reply</Button>
      </QuietVerbs>,
    );
    expect(face("Draft reply")).toContain("btn-ai");
  });
});

describe("an AI verb draws its own mark", () => {
  it("draws the ✦ on ai and aiQuiet, filled or quieted", () => {
    render(
      <>
        <Button variant="ai">Draft reply</Button>
        <Button variant="aiQuiet">Shorter</Button>
        <QuietVerbs>
          <Button variant="ai">Prepare</Button>
        </QuietVerbs>
        <Button variant="primary">Save</Button>
      </>,
    );
    expect(marked("Draft reply")).toBe(true);
    expect(marked("Shorter")).toBe(true);
    expect(marked("Prepare")).toBe(true);
    expect(marked("Save")).toBe(false);
  });

  // The button draws the mark, so a caller that hands it one draws two. A
  // census over the tree, because the second mark is in some screen's JSX.
  it("is never handed a second one by a caller", { timeout: 60_000 }, () => {
    const root = join(here, "..");
    const files = filesMatching(root, /\.tsx$/).concat(
      extensionFrontendFiles(join(root, "..", "..", "extensions")),
    );
    expect(files.length).toBeGreaterThan(100);
    const found: string[] = [];
    for (const file of files) {
      const source = sourceFileAt(file);
      const walk = (node: ts.Node, inAiButton: boolean) => {
        let inside = inAiButton;
        if (ts.isJsxElement(node) && isAiButton(node.openingElement)) {
          inside = true;
        }
        if (inside && isSparkles(node)) {
          const { line } = source.getLineAndCharacterOfPosition(
            node.getStart(),
          );
          found.push(`${relative(root, file)}:${line + 1}`);
        }
        node.forEachChild((child) => walk(child, inside));
      };
      walk(source, false);
    }
    expect(found).toEqual([]);
  });
});

// A computed variant counts as AI, since a lookup can come out as "ai".
// Only a literal naming another variant is known to draw no ✦.
function isAiButton(open: ts.JsxOpeningElement): boolean {
  if (open.tagName.getText() !== "Button") return false;
  return open.attributes.properties.some(
    (attribute) =>
      ts.isJsxAttribute(attribute) &&
      attribute.name.getText() === "variant" &&
      !namesPlainVariant(attribute.initializer),
  );
}

function namesPlainVariant(value: ts.JsxAttributeValue | undefined): boolean {
  const literal = value && ts.isJsxExpression(value) ? value.expression : value;
  return (
    literal !== undefined &&
    ts.isStringLiteral(literal) &&
    !/^ai(Quiet)?$/.test(literal.text)
  );
}

function isSparkles(node: ts.Node): boolean {
  const open = ts.isJsxElement(node)
    ? node.openingElement
    : ts.isJsxSelfClosingElement(node)
      ? node
      : undefined;
  return open?.tagName.getText() === "Sparkles";
}
