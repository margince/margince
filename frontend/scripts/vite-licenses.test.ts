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
import { build } from "vite";
import { describe, expect, it } from "vitest";
import spaConfig from "../vite.config";
import { mcpAppView } from "../vite.mcp-apps.config";
import { inlineViews } from "./vite-inline-views";
import { THIRD_PARTY_NOTICES, thirdPartyNotices } from "./vite-licenses";

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

async function fixtureBuild(root: string, inline = false, view = false) {
  const config = view ? mcpAppView("create-followups", join(root, "dist")) : {};
  const result = await build({
    ...config,
    configFile: false,
    root,
    logLevel: "silent",
    plugins: [
      thirdPartyNotices({ repository: root, inline }),
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

describe("third-party notices", () => {
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

  it("names the core packages in the actual SPA build", async () => {
    const result = await build({
      ...spaConfig,
      configFile: false,
      root: FRONTEND,
      logLevel: "silent",
      build: { ...spaConfig.build, write: false },
    });
    const outputs = (Array.isArray(result) ? result : [result]).flatMap(
      (bundle) => ("output" in bundle ? bundle.output : []),
    );
    const notice = outputs.find(
      (item) => item.fileName === THIRD_PARTY_NOTICES,
    );
    const text = notice && "source" in notice ? String(notice.source) : "";
    for (const name of ["react", "react-dom", "lucide-react", "tailwindcss"])
      expect(text).toContain(`${name}@`);
  }, 30_000);
});
