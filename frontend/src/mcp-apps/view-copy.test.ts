import {
  mkdirSync,
  mkdtempSync,
  readdirSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { viewCopy } from "./view-copy.testkit";

const viewsDir = dirname(fileURLToPath(import.meta.url));

describe("viewCopy", () => {
  it("reads the title and the main module of every view", () => {
    const keys = Object.keys(viewCopy());
    const views = readdirSync(viewsDir, { withFileTypes: true })
      .filter((entry) => entry.isDirectory())
      .map((entry) => entry.name);
    expect(views.length).toBeGreaterThan(0);
    for (const view of views) {
      expect(keys).toContain(`mcp-apps/${view}/index.html:<title>`);
      expect(
        keys.some((key) => key.startsWith(`mcp-apps/${view}/main.ts:`)),
      ).toBe(true);
    }
  });

  it("keeps every string, writes a substitution as {} and skips test code", () => {
    const dir = mkdtempSync(join(tmpdir(), "view-copy-"));
    mkdirSync(join(dir, "card"));
    writeFileSync(
      join(dir, "card", "main.ts"),
      `import "./view.css";\nconst a = ["Open", \`Due \${when} late\`];\n`,
    );
    writeFileSync(join(dir, "card", "main.test.ts"), 'it("don\'t");\n');
    writeFileSync(join(dir, "card", "fixture.ts"), 'const f = "we";\n');
    writeFileSync(
      join(dir, "card", "index.html"),
      "<html><title>The card</title></html>",
    );
    try {
      expect(viewCopy(dir)).toEqual({
        "mcp-apps/card/main.ts:2:12": "Open",
        "mcp-apps/card/main.ts:2:20": "Due {} late",
        "mcp-apps/card/index.html:<title>": "The card",
      });
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});
