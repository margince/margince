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

  it("keeps every string and label, writes a substitution as {} and skips test and story code", () => {
    const dir = mkdtempSync(join(tmpdir(), "view-copy-"));
    mkdirSync(join(dir, "card"));
    writeFileSync(
      join(dir, "card", "main.ts"),
      `import "./view.css";\nconst a = ["Open", \`Due \${when} late\`];\n`,
    );
    writeFileSync(join(dir, "card", "main.test.ts"), 'it("don\'t");\n');
    writeFileSync(join(dir, "card", "fixture.ts"), 'const f = "we";\n');
    writeFileSync(join(dir, "card", "card.stories.tsx"), 'const s = "we";\n');
    writeFileSync(join(dir, "story-hosts.tsx"), 'const h = "we";\n');
    writeFileSync(
      join(dir, "card", "panel.tsx"),
      'const p = <p aria-label="Close panel">  Saved  </p>;\n',
    );
    writeFileSync(
      join(dir, "card", "index.html"),
      '<html><title>The card</title>\n<img alt="Logo" /><input placeholder="Search" aria-label="Find" title="Hint" /></html>',
    );
    try {
      expect(viewCopy(dir)).toEqual({
        "mcp-apps/card/main.ts:2:12": "Open",
        "mcp-apps/card/main.ts:2:20": "Due {} late",
        "mcp-apps/card/panel.tsx:1:25": "Close panel",
        "mcp-apps/card/panel.tsx:1:41": "Saved",
        "mcp-apps/card/index.html:<title>": "The card",
        "mcp-apps/card/index.html:2:alt": "Logo",
        "mcp-apps/card/index.html:2:placeholder": "Search",
        "mcp-apps/card/index.html:2:aria-label": "Find",
        "mcp-apps/card/index.html:2:title": "Hint",
      });
    } finally {
      rmSync(dir, { recursive: true, force: true });
    }
  });
});
