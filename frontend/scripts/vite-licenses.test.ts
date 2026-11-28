// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { build, type Plugin } from "vite";
import { describe, expect, it } from "vitest";
import spaConfig from "../vite.config";
import { mcpAppView } from "../vite.mcp-apps.config";
import { inlineViews } from "./vite-inline-views";
import {
  SPA_CORE_PACKAGES,
  THIRD_PARTY_NOTICES,
  thirdPartyNotices,
} from "./vite-licenses";

const FRONTEND = resolve(dirname(fileURLToPath(import.meta.url)), "..");

function fixture(license: boolean) {
  const root = mkdtempSync(join(tmpdir(), "margince-notices-"));
  const pkg = join(root, "node_modules/fake-pkg");
  mkdirSync(pkg, { recursive: true });
  writeFileSync(
    join(pkg, "package.json"),
    JSON.stringify({
      name: "fake-pkg",
      version: "1.2.3",
      license: "MIT",
      main: "index.js",
    }),
  );
  writeFileSync(join(pkg, "index.js"), "export const value = 'hello';");
  if (license)
    writeFileSync(
      join(pkg, "LICENSE"),
      "Copyright example\nPermission granted.",
    );
  writeFileSync(join(root, "third-party-files.txt"), "");
  writeFileSync(
    join(root, "index.html"),
    '<!doctype html><html><head></head><body><script type="module" src="/entry.js"></script></body></html>',
  );
  writeFileSync(
    join(root, "entry.js"),
    "import {value} from 'fake-pkg';document.body.textContent=value;",
  );
  return root;
}

async function fixtureBuild(
  root: string,
  inline = false,
  view = false,
  named: readonly string[] = [],
) {
  const config = view ? mcpAppView("create-followups", join(root, "dist")) : {};
  const result = await build({
    ...config,
    configFile: false,
    root,
    logLevel: "silent",
    plugins: [
      thirdPartyNotices({ repository: root, inline, named }),
      ...(view ? [inlineViews()] : []),
    ],
    build: {
      ...config.build,
      write: false,
      rolldownOptions: {
        ...config.build?.rolldownOptions,
        input: join(root, "index.html"),
      },
    },
  });
  return (Array.isArray(result) ? result : [result]).flatMap((bundle) =>
    "output" in bundle ? bundle.output : [],
  );
}

function noticesPlugin(option: unknown): Plugin | undefined {
  if (Array.isArray(option)) {
    return option.map(noticesPlugin).find((plugin) => plugin !== undefined);
  }
  return isPlugin(option) && option.name === "margince-third-party-notices"
    ? option
    : undefined;
}

function isPlugin(option: unknown): option is Plugin {
  return (
    typeof option === "object" &&
    option !== null &&
    "name" in option &&
    typeof option.name === "string"
  );
}

function withDownloadedUnit(root: string, license: boolean) {
  const unit = join(root, "build/extensions/acme");
  mkdirSync(unit, { recursive: true });
  writeFileSync(join(unit, "screen.js"), "export const screen = 'acme';");
  if (license) writeFileSync(join(unit, "LICENSE"), "ACME-UNIT-LICENCE-TEXT");
  writeFileSync(
    join(root, "build/extensions/fetched-units.txt"),
    "acme | example.test/acme/crm/v2@v2.1.0\n",
  );
  writeFileSync(
    join(root, "entry.js"),
    "import {value} from 'fake-pkg';import {screen} from './build/extensions/acme/screen.js';document.body.textContent=value+screen;",
  );
}

describe("third-party notices", () => {
  it("ships a downloaded unit's own licence text, and refuses a unit without one", async () => {
    const root = fixture(true);
    try {
      withDownloadedUnit(root, true);
      const outputs = await fixtureBuild(root);
      const notice = outputs.find(
        (item) => item.fileName === THIRD_PARTY_NOTICES,
      );
      const text = notice && "source" in notice ? String(notice.source) : "";
      expect(text).toContain("example.test/acme/crm/v2@v2.1.0");
      expect(text).toContain("ACME-UNIT-LICENCE-TEXT");
      rmSync(join(root, "build/extensions/acme/LICENSE"));
      await expect(fixtureBuild(root)).rejects.toThrow(/acme\/crm.*no LICENSE/);
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });

  it("ships and checks the licence of a unit the bundle reaches only through CSS", async () => {
    const root = fixture(true);
    try {
      withDownloadedUnit(root, true);
      writeFileSync(
        join(root, "build/extensions/acme/style.css"),
        "a{color:red}",
      );
      writeFileSync(
        join(root, "entry.js"),
        "import {value} from 'fake-pkg';import './build/extensions/acme/style.css';document.body.textContent=value;",
      );
      const outputs = await fixtureBuild(root);
      const notice = outputs.find(
        (item) => item.fileName === THIRD_PARTY_NOTICES,
      );
      expect(
        notice && "source" in notice ? String(notice.source) : "",
      ).toContain("ACME-UNIT-LICENCE-TEXT");
      rmSync(join(root, "build/extensions/acme/LICENSE"));
      await expect(fixtureBuild(root)).rejects.toThrow(/acme\/crm.*no LICENSE/);
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });

  it("refuses downloaded unit folders that have no index", async () => {
    const root = fixture(true);
    try {
      withDownloadedUnit(root, true);
      rmSync(join(root, "build/extensions/fetched-units.txt"));
      await expect(fixtureBuild(root)).rejects.toThrow(
        /fetched-units.txt.*missing/,
      );
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });

  it("leaves out a downloaded unit that the bundle does not ship", async () => {
    const root = fixture(true);
    try {
      withDownloadedUnit(root, true);
      writeFileSync(
        join(root, "entry.js"),
        "import {value} from 'fake-pkg';document.body.textContent=value;",
      );
      const outputs = await fixtureBuild(root);
      const notice = outputs.find(
        (item) => item.fileName === THIRD_PARTY_NOTICES,
      );
      expect(
        notice && "source" in notice ? String(notice.source) : "",
      ).not.toContain("ACME-UNIT-LICENCE-TEXT");
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });

  it("fails the real bundle when a bundled package has no licence text", async () => {
    const root = fixture(false);
    try {
      await expect(fixtureBuild(root)).rejects.toThrow(
        /fake-pkg@1.2.3.*no licence text/,
      );
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });

  it("uses recorded licence text only for the exact bundled package version", async () => {
    const root = fixture(false);
    try {
      mkdirSync(join(root, "vendor"));
      writeFileSync(
        join(root, "vendor/permission.txt"),
        "Recorded upstream permission",
      );
      writeFileSync(
        join(root, "third-party-files.txt"),
        "vendor/permission.txt | fake-pkg@1.2.3 | MIT | vendor/permission.txt\n",
      );
      const outputs = await fixtureBuild(root);
      const notice = outputs.find(
        (item) => item.fileName === THIRD_PARTY_NOTICES,
      );
      expect(
        notice && "source" in notice ? String(notice.source) : "",
      ).toContain("Recorded upstream permission");
      writeFileSync(
        join(root, "third-party-files.txt"),
        "vendor/permission.txt | fake-pkg@1.2.2 | MIT | vendor/permission.txt\n",
      );
      await expect(fixtureBuild(root)).rejects.toThrow(
        /fake-pkg@1.2.3.*no licence text/,
      );
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });

  it("ships the licence and notice text with the package version", async () => {
    const root = fixture(true);
    try {
      writeFileSync(
        join(root, "node_modules/fake-pkg/NOTICE"),
        "Additional copyright notice",
      );
      const outputs = await fixtureBuild(root);
      const notice = outputs.find(
        (item) => item.fileName === THIRD_PARTY_NOTICES,
      );
      expect(notice?.type).toBe("asset");
      const text = notice && "source" in notice ? String(notice.source) : "";
      expect(text).toContain("fake-pkg@1.2.3\nMIT");
      expect(text).toContain(
        readFileSync(join(root, "node_modules/fake-pkg/LICENSE"), "utf8"),
      );
      expect(text).toContain("Additional copyright notice");
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });

  it("includes loaded copied files and refuses a missing copied licence", async () => {
    const root = fixture(true);
    try {
      mkdirSync(join(root, "copied"));
      writeFileSync(join(root, "copied/icon.js"), "export const icon='mark';");
      writeFileSync(
        join(root, "entry.js"),
        "import {icon} from './copied/icon.js';document.body.textContent=icon;",
      );
      writeFileSync(
        join(root, "third-party-files.txt"),
        "copied/icon.js | example 1 | MIT | copied/LICENSE\n",
      );
      await expect(fixtureBuild(root)).rejects.toThrow(/copied\/LICENSE/);
      writeFileSync(
        join(root, "copied/LICENSE"),
        "The copied mark's permission",
      );
      const outputs = await fixtureBuild(root);
      const notice = outputs.find(
        (item) => item.fileName === THIRD_PARTY_NOTICES,
      );
      expect(
        notice && "source" in notice ? String(notice.source) : "",
      ).toContain("The copied mark's permission");
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });

  it("embeds complete notices when the consumer must remain one HTML document", async () => {
    const root = fixture(true);
    try {
      writeFileSync(join(root, "entry.css"), "body{color:red}");
      writeFileSync(
        join(root, "entry.js"),
        "import './entry.css'; import {value} from 'fake-pkg';document.body.textContent=value;",
      );
      const license =
        "Copyright example\nPermission granted. https://example.test/license <not markup>";
      writeFileSync(join(root, "node_modules/fake-pkg/LICENSE"), license);
      const outputs = await fixtureBuild(root, true, true);
      expect(
        outputs.some((item) => item.fileName === THIRD_PARTY_NOTICES),
      ).toBe(false);
      const html = outputs.find((item) => item.fileName.endsWith(".html"));
      const source = html && "source" in html ? String(html.source) : "";
      expect(source).toContain('id="third-party-notices"');
      const encoded = source.match(
        /<script type="application\/json" id="third-party-notices">([^<]*)<\/script>/,
      )?.[1];
      expect(encoded).toBeDefined();
      const decoded = JSON.parse(encoded ?? "null");
      expect(decoded).toContain("fake-pkg@1.2.3");
      expect(decoded).toContain(license);
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });

  it("covers every Vite config reached by the package build command", () => {
    const pkg = JSON.parse(
      readFileSync(join(FRONTEND, "package.json"), "utf8"),
    );
    const scripts: Record<string, string> = pkg.scripts;
    const visited = new Set<string>();
    const configs = new Set<string>();
    function visit(name: string) {
      if (visited.has(name)) return;
      visited.add(name);
      const command = scripts[name];
      expect(command).toBeDefined();
      for (const match of command.matchAll(/\bpnpm\s+(?:run\s+)?([\w:-]+)/g))
        visit(match[1]);
      for (const match of command.matchAll(
        /\bvite build(?:\s+--config\s+([^\s]+))?/g,
      ))
        configs.add(match[1] ?? "vite.config.ts");
      for (const match of command.matchAll(/\bnode\s+(scripts\/[^\s]+)/g)) {
        const driver = readFileSync(join(FRONTEND, match[1]), "utf8");
        const found = [...driver.matchAll(/vite[^\s"']*\.config\.ts/g)];
        expect(found.length).toBeGreaterThan(0);
        for (const config of found) configs.add(config[0]);
      }
    }
    visit("build");
    expect(configs.size).toBeGreaterThan(0);
    for (const config of configs)
      expect(readFileSync(join(FRONTEND, config), "utf8")).toMatch(
        /thirdPartyNotices\(/,
      );
  });

  it("refuses a build that ships no module of a package it names", async () => {
    const root = fixture(true);
    try {
      const outputs = await fixtureBuild(root, false, false, ["fake-pkg"]);
      expect(
        outputs.some((item) => item.fileName === THIRD_PARTY_NOTICES),
      ).toBe(true);
      await expect(
        fixtureBuild(root, false, false, ["fake-pkg", "react"]),
      ).rejects.toThrow(/no bundled module of react, which/);
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });

  it("refuses an SPA build that bundles none of the core packages", async () => {
    const plugin = noticesPlugin(spaConfig.plugins);
    expect(plugin).toBeDefined();
    const root = fixture(true);
    try {
      await expect(
        build({
          configFile: false,
          root,
          logLevel: "silent",
          plugins: [plugin],
          build: {
            write: false,
            rolldownOptions: { input: join(root, "index.html") },
          },
        }),
      ).rejects.toThrow(
        `no bundled module of ${SPA_CORE_PACKAGES.join(", ")}, which`,
      );
    } finally {
      rmSync(root, { recursive: true, force: true });
    }
  });
});
