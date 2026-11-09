// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  existsSync,
  readdirSync,
  readFileSync,
  realpathSync,
  statSync,
} from "node:fs";
import { dirname, relative, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import {
  type Plugin,
  preprocessCSS,
  type ResolvedConfig,
  type Rolldown,
} from "vite";

const REPOSITORY = resolve(dirname(fileURLToPath(import.meta.url)), "../..");
export const THIRD_PARTY_NOTICES = "licenses/THIRD-PARTY-NOTICES.txt";

type CopiedFile = {
  path: string;
  source: string;
  license: string;
  text: string;
};
type BundledPackage = {
  root: string;
  name: string;
  version: string;
  license: string;
};

// The web image has no Python; this reader validates the same manifest contract as the SBOM checker.
function copiedFiles(root: string): CopiedFile[] {
  const seen = new Set<string>();
  return readFileSync(resolve(root, "third-party-files.txt"), "utf8")
    .split(/\r?\n/)
    .filter((line) => line.trim() && !line.trim().startsWith("#"))
    .map((line) => {
      const fields = line.split("|").map((field) => field.trim());
      if (fields.length !== 4 || fields.some((field) => !field)) {
        throw new Error(`third-party-files.txt: expected four fields: ${line}`);
      }
      for (const path of [fields[0], fields[3]]) {
        if (path.startsWith("/") || path.split("/").includes(".."))
          throw new Error(
            "third-party-files.txt: path must stay inside the repository",
          );
      }
      if (seen.has(fields[0]))
        throw new Error(
          `third-party-files.txt: duplicate copied path ${fields[0]}`,
        );
      seen.add(fields[0]);
      return {
        path: fields[0],
        source: fields[1],
        license: fields[2],
        text: fields[3],
      };
    });
}

function moduleFile(id: string): string {
  return id.replace(/^\0/, "").split("?")[0];
}

function bundledPackage(id: string): BundledPackage | undefined {
  const file = moduleFile(id);
  if (!file.includes("/node_modules/")) return undefined;
  let folder = dirname(file);
  while (dirname(folder) !== folder) {
    const manifest = resolve(folder, "package.json");
    if (existsSync(manifest)) {
      const value: unknown = JSON.parse(readFileSync(manifest, "utf8"));
      if (
        typeof value === "object" &&
        value !== null &&
        "name" in value &&
        typeof value.name === "string" &&
        "version" in value &&
        typeof value.version === "string"
      ) {
        const license =
          "license" in value && typeof value.license === "string"
            ? value.license
            : "NOASSERTION";
        return {
          root: folder,
          name: value.name,
          version: value.version,
          license,
        };
      }
    }
    folder = dirname(folder);
  }
  throw new Error(
    `third-party notices: bundled module has no package manifest: ${file}`,
  );
}

function noticeText(path: string): string {
  const text = readFileSync(path, "utf8");
  if (!text.trim())
    throw new Error(`third-party notices: empty licence text: ${path}`);
  return text;
}

function packageNotice(
  pkg: BundledPackage,
  copied: CopiedFile[],
  root: string,
): string {
  const files = readdirSync(pkg.root)
    .filter(
      (name) =>
        /^(?:licen[cs]e|notice|copying)(?:[._-].*)?$/i.test(name) &&
        statSync(resolve(pkg.root, name)).isFile(),
    )
    .sort();
  const texts = files.map((name) => noticeText(resolve(pkg.root, name)));
  if (texts.length === 0) {
    const recorded = copied.find(
      (entry) => entry.source === `${pkg.name}@${pkg.version}`,
    );
    if (recorded) texts.push(noticeText(resolve(root, recorded.text)));
  }
  if (texts.length === 0) {
    throw new Error(
      `third-party notices: ${pkg.name}@${pkg.version} has no licence text; record its provenance and licence text in third-party-files.txt`,
    );
  }
  return `${pkg.name}@${pkg.version}\n${pkg.license}\n\n${texts.join("\n\n")}`;
}

function bundleNotices(
  root: string,
  bundle: Rolldown.OutputBundle,
  cssFiles: readonly string[],
): string {
  const copied = copiedFiles(root);
  const packages = new Map<string, BundledPackage>();
  const loaded = new Set<string>();
  for (const output of Object.values(bundle)) {
    if (output.type !== "chunk") continue;
    for (const id of Object.keys(output.modules)) {
      loaded.add(relative(root, moduleFile(id)));
      const pkg = bundledPackage(id);
      if (pkg) packages.set(pkg.root, pkg);
    }
  }
  for (const file of cssFiles.filter((file) => file.endsWith(".css"))) {
    const pkg = bundledPackage(file);
    if (pkg) packages.set(pkg.root, pkg);
  }
  const notices = [...packages.values()]
    .sort((a, b) =>
      `${a.name}@${a.version}`.localeCompare(`${b.name}@${b.version}`),
    )
    .map((pkg) => packageNotice(pkg, copied, root));
  const copiedNotices = new Map<string, string>();
  for (const entry of copied) {
    if (loaded.has(entry.path)) {
      copiedNotices.set(
        entry.text,
        `${entry.source}\n${entry.license}\n\n${noticeText(resolve(root, entry.text))}`,
      );
    }
  }
  return `${[...notices, ...copiedNotices.values()].join("\n\n====================\n\n")}\n`;
}

// CSS imports are inlined outside chunk.modules. Vite reports their resolved
// files without maintaining a second CSS import parser here.
async function cssDependencies(
  ids: IterableIterator<string>,
  config: ResolvedConfig,
): Promise<string[]> {
  const dependencies = new Set<string>();
  for (const id of ids) {
    const file = moduleFile(id);
    if (!file.endsWith(".css") || !existsSync(file)) continue;
    const result = await preprocessCSS(
      readFileSync(file, "utf8"),
      file,
      config,
    );
    dependencies.add(file);
    for (const dependency of result.deps ?? []) dependencies.add(dependency);
  }
  return [...dependencies];
}

// JSON escapes keep licence URLs inert under the shared document network validator.
function noticeJSON(text: string): string {
  return (
    '"' +
    text
      .split("")
      .map(
        (letter) => `\\u${letter.charCodeAt(0).toString(16).padStart(4, "0")}`,
      )
      .join("") +
    '"'
  );
}

function embedNotices(bundle: Rolldown.OutputBundle, text: string): void {
  // MCP views travel as one HTML document, so their notices travel inside it.
  let embedded = false;
  for (const output of Object.values(bundle)) {
    if (output.type === "asset" && output.fileName.endsWith(".html")) {
      embedded = true;
      const html =
        typeof output.source === "string"
          ? output.source
          : new TextDecoder().decode(output.source);
      output.source = `${html}\n<script type="application/json" id="third-party-notices">${noticeJSON(text)}</script>\n`;
    }
  }
  if (!embedded)
    throw new Error(
      "third-party notices: inline build emitted no HTML document",
    );
}

/** The emitted chunks decide which packages ship; the lockfile also holds build tools. */
export function thirdPartyNotices(
  options: { repository?: string; inline?: boolean } = {},
): Plugin {
  const root = realpathSync(options.repository ?? REPOSITORY);
  let config: ResolvedConfig | undefined;
  return {
    name: "margince-third-party-notices",
    apply: "build",
    enforce: "post",
    configResolved(resolved) {
      config = resolved;
    },
    async generateBundle(_options, bundle) {
      if (!options.inline && !("index.html" in bundle)) return;
      if (!config)
        this.error("third-party notices: build configuration is missing");
      const css = await cssDependencies(this.getModuleIds(), config);
      const text = bundleNotices(root, bundle, css);
      if (options.inline) {
        embedNotices(bundle, text);
      } else {
        this.emitFile({
          type: "asset",
          fileName: THIRD_PARTY_NOTICES,
          source: text,
        });
      }
    },
  };
}
