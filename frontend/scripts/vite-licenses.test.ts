// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { build } from "vite";
import { describe, expect, it } from "vitest";
import spaConfig from "../vite.config";
import {
  MATERIAL_SYMBOLS_LICENSE_SOURCE,
  materialSymbolsLicense,
} from "./vite-licenses";

describe("materialSymbolsLicense", () => {
  it("emits the source LICENSE byte for byte into a build", async () => {
    const root = mkdtempSync(join(tmpdir(), "margince-licenses-"));
    try {
      writeFileSync(join(root, "index.html"), "<!doctype html><title>t</title>");
      const result = await build({
        configFile: false,
        root,
        logLevel: "warn",
        plugins: [materialSymbolsLicense()],
        build: { write: false },
      });
      const outputs = (Array.isArray(result) ? result : [result]).flatMap(
        (bundle) => ("output" in bundle ? bundle.output : []),
      );
      const emitted = outputs.find(
        (item) => item.fileName === "licenses/material-symbols.txt",
      );
      expect(emitted?.type).toBe("asset");
      expect(emitted && "source" in emitted ? String(emitted.source) : "").toBe(
        readFileSync(MATERIAL_SYMBOLS_LICENSE_SOURCE, "utf8"),
      );
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });

  it("is part of the SPA build", () => {
    const names = (spaConfig.plugins ?? [])
      .flat()
      .map((plugin) =>
        typeof plugin === "object" && plugin !== null && "name" in plugin
          ? plugin.name
          : "",
      );
    expect(names).toContain("margince-material-symbols-license");
  });
});
