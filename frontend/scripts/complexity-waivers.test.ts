import { readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";

// A waiver of the complexity or function-length rule in production code names
// the issue that splits the function. Without one, the waiver is where an
// oversized function stops being anybody's work. Test code carries no cap.

const here = dirname(fileURLToPath(import.meta.url));
const srcDir = join(here, "..", "src");

const WAIVER =
  /biome-ignore\s+[^:]*lint\/complexity\/noExcessive(?:CognitiveComplexity|LinesPerFunction)[^:]*:(.*)$/;
const ISSUE = /#\d+\b|\/issues\/\d+\b/;
const TEST_CODE = /\.(test|testkit|stories)\.tsx?$|\/testing\//;

function sourceFiles(dir: string): string[] {
  return readdirSync(dir, { withFileTypes: true }).flatMap((entry) => {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) return sourceFiles(path);
    return /\.tsx?$/.test(entry.name) ? [path] : [];
  });
}

// unnamedWaivers lists each complexity waiver whose reason names no issue.
function unnamedWaivers(source: string): number[] {
  return source.split("\n").flatMap((line, i) => {
    const waiver = WAIVER.exec(line);
    return waiver && !ISSUE.test(waiver[1]) ? [i + 1] : [];
  });
}

describe("a complexity waiver names the issue that splits the function", () => {
  it("reads the whole source tree", () => {
    expect(sourceFiles(srcDir).length).toBeGreaterThan(1000);
  });

  it("finds no production waiver without an issue", () => {
    const production = sourceFiles(srcDir).filter((f) => !TEST_CODE.test(f));
    const missing = production.flatMap((file) =>
      unnamedWaivers(readFileSync(file, "utf8")).map(
        (line) => `${relative(srcDir, file)}:${line}`,
      ),
    );
    expect(missing).toEqual([]);
  });

  it("tells a named waiver from an unnamed one", () => {
    const named =
      "// biome-ignore lint/complexity/noExcessiveLinesPerFunction: splitting it is #7201";
    const both =
      "// biome-ignore lint/complexity/noExcessiveCognitiveComplexity lint/complexity/noExcessiveLinesPerFunction: one surface";
    const other =
      "// biome-ignore lint/suspicious/noArrayIndexKey: the position is the identity";
    expect(unnamedWaivers(`${named}\n${both}\n${other}`)).toEqual([2]);
  });
});
