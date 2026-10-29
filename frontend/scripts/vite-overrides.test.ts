// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { build } from "vite";
import { describe, expect, it } from "vitest";
import spaConfig from "../vite.config";
import { overrideCoreFiles, readFileOverrides } from "./vite-overrides";

// A core with a menu that main.ts imports, and a unit file replacing the menu
// that wraps core's own copy.
function coreAndUnit(): {
  core: string;
  unitFile: string;
  cleanup: () => void;
} {
  const root = mkdtempSync(join(tmpdir(), "margince-overrides-"));
  const core = join(root, "frontend");
  mkdirSync(join(core, "src", "app"), { recursive: true });
  writeFileSync(
    join(core, "src", "app", "menu.ts"),
    'export const items = ["core row"];\n',
  );
  writeFileSync(
    join(core, "src", "main.ts"),
    'import { items } from "./app/menu";\nconsole.log(items.join("|"));\n',
  );
  const unitFile = join(root, "unit", "menu.ts");
  mkdirSync(join(root, "unit"));
  writeFileSync(
    unitFile,
    'import { items as core } from "@margince-core/src/app/menu";\nexport const items = [...core, "unit row"];\n',
  );
  return {
    core,
    unitFile,
    cleanup: () => rmSync(root, { recursive: true, force: true }),
  };
}

async function bundle(core: string, unitFile: string | undefined) {
  const result = await build({
    configFile: false,
    root: core,
    logLevel: "warn",
    plugins: [
      overrideCoreFiles(
        core,
        unitFile
          ? [{ unit: "acme", target: "src/app/menu.ts", source: unitFile }]
          : [],
      ),
    ],
    build: {
      write: false,
      minify: false,
      lib: { entry: join(core, "src", "main.ts"), formats: ["es"] },
    },
  });
  return (Array.isArray(result) ? result : [result])
    .flatMap((b) => ("output" in b ? b.output : []))
    .map((item) => ("code" in item ? item.code : ""))
    .join("\n");
}

describe("overrideCoreFiles", () => {
  it("serves the unit's file to every importer, and core's own copy to the unit", async () => {
    const { core, unitFile, cleanup } = coreAndUnit();
    try {
      const code = await bundle(core, unitFile);
      expect(code).toContain("core row");
      expect(code).toContain("unit row");
    } finally {
      cleanup();
    }
  });

  it("leaves core alone when nothing is replaced", async () => {
    const { core, cleanup } = coreAndUnit();
    try {
      const code = await bundle(core, undefined);
      expect(code).toContain("core row");
      expect(code).not.toContain("unit row");
    } finally {
      cleanup();
    }
  });

  it("reads no replacement outside a composed lane", () => {
    expect(readFileOverrides(__filename, false)).toEqual([]);
  });

  it("refuses a composed lane whose overrides file was never written", () => {
    expect(() =>
      readFileOverrides(join(tmpdir(), "absent-overrides.json"), true),
    ).toThrow("make -C backend composition");
  });

  it("is part of the SPA build", () => {
    const names = (spaConfig.plugins ?? [])
      .flat()
      .map((plugin) =>
        typeof plugin === "object" && plugin !== null && "name" in plugin
          ? plugin.name
          : "",
      );
    expect(names).toContain("margince-core-overrides");
  });
});
