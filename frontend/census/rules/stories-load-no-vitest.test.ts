// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { plantedFindings } from "../tree";
import { rule } from "./stories-load-no-vitest.census";

// The import walk reads files from disk, so the planted modules are written
// to a temporary directory.
describe("the stories-load-no-vitest rule", () => {
  let dir = "";
  const plant = (name: string, text: string) => {
    const path = join(dir, name);
    mkdirSync(dir, { recursive: true });
    writeFileSync(path, text);
    return { path, area: "src" as const, text };
  };

  beforeEach(() => {
    dir = mkdtempSync(join(tmpdir(), "stories-load-no-vitest-"));
  });
  afterEach(() => {
    rmSync(dir, { recursive: true, force: true });
  });

  it("reports a story that reaches vitest through a helper", () => {
    const findings = plantedFindings(rule, [
      plant("card.stories.tsx", 'import { kit } from "./kit";\nkit();\n'),
      plant(
        "kit.ts",
        'import { vi } from "vitest";\nexport const kit = vi.fn;\n',
      ),
    ]);
    expect(findings).toHaveLength(1);
    expect(findings[0]).toMatch(/card\.stories\.tsx -> .*kit\.ts$/);
  });

  it("passes a story whose helper reaches vitest by type only", () => {
    expect(
      plantedFindings(rule, [
        plant("card.stories.tsx", 'import type { Kit } from "./kit";\n'),
        plant(
          "kit.ts",
          'import { vi } from "vitest";\nexport type Kit = typeof vi;\n',
        ),
      ]),
    ).toEqual([]);
  });

  it("reads a docs page and leaves a file Storybook never loads alone", () => {
    const helper = plant("kit.ts", 'import "vitest";\n');
    expect(
      plantedFindings(rule, [
        plant("card.mdx", 'import { kit } from "./kit";\n\n# Card\n'),
        plant("card.test.tsx", 'import { kit } from "./kit";\n'),
        helper,
      ]),
    ).toHaveLength(1);
  });
});
