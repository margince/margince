// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { build, type Plugin, type Rolldown, runnerImport } from "vite";

const FRONTEND = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const WORKER_SOURCE = resolve(FRONTEND, "src/offline/serviceworker.js");

const OFFLINE_PAGE = "/offline.html";

type PwaFile = Readonly<{ fileName: string; source: string }>;

type ServiceWorkerSettings = Readonly<{
  cacheName: string;
  offlinePage: string;
  offlineScript: string;
  passThrough: readonly string[];
}>;

// A line of its own ahead of the untouched source, so the tests run that source
// file as it is and coverage lands on it.
function renderServiceWorker(settings: ServiceWorkerSettings): string {
  const source = readFileSync(WORKER_SOURCE, "utf8");
  return `self.__MARGINCE_SW_SETTINGS__ = ${JSON.stringify(settings)};\n${source}`;
}

type OfflineScript = Readonly<{ fileName: string; code: string; css: string }>;

// Its own build, not a second entry of the SPA's: a module both entries import
// would be split into a chunk the offline page then has to fetch.
async function buildOfflineScript(): Promise<OfflineScript> {
  const result = await build({
    configFile: false,
    root: FRONTEND,
    publicDir: false,
    logLevel: "warn",
    build: {
      write: false,
      modulePreload: false,
      rolldownOptions: {
        input: resolve(FRONTEND, "src/offline/entry.ts"),
        output: {
          entryFileNames: "assets/offline-[hash].js",
          assetFileNames: "assets/offline-[hash][extname]",
        },
      },
    },
  });
  const outputs = (Array.isArray(result) ? result : [result]).flatMap(
    (bundle) => ("output" in bundle ? bundle.output : []),
  );
  const chunks = outputs.filter(
    (item): item is Rolldown.OutputChunk => item.type === "chunk",
  );
  const sheets = outputs.filter(
    (item): item is Rolldown.OutputAsset =>
      item.type === "asset" && item.fileName.endsWith(".css"),
  );
  const [chunk] = chunks;
  const [sheet] = sheets;
  if (outputs.length !== 2 || chunk === undefined || sheet === undefined) {
    throw new Error(
      `the offline page built ${outputs.map((item) => item.fileName).join(", ")}; ` +
        "it must be one script and one stylesheet, because the page can fetch nothing else",
    );
  }
  return {
    fileName: chunk.fileName,
    code: chunk.code,
    css: String(sheet.source),
  };
}

/** The cache is keyed on the release and a digest of what it holds, so each
 *  change gives the worker new bytes and every browser installs it afresh. */
async function buildPwaFiles(
  options: Readonly<{ release: string; passThrough: readonly string[] }>,
): Promise<readonly PwaFile[]> {
  const script = await buildOfflineScript();
  const offlineScript = `/${script.fileName}`;
  // Through Vite's runner, so the catalogs it reads never join the config's own graph.
  const { module: offline } = await runnerImport<
    typeof import("../src/offline/page.ts")
  >(resolve(FRONTEND, "src/offline/page.ts"), {
    configFile: false,
    root: FRONTEND,
    logLevel: "warn",
  });
  const page = offline.renderOfflinePage(script.css, offlineScript);
  const digest = createHash("sha256")
    .update(page)
    .update(readFileSync(WORKER_SOURCE))
    .update(JSON.stringify(options.passThrough))
    .digest("hex")
    .slice(0, 12);
  const worker = renderServiceWorker({
    cacheName: `margince-offline-${options.release || "dev"}-${digest}`,
    offlinePage: OFFLINE_PAGE,
    offlineScript,
    passThrough: options.passThrough,
  });
  return [
    { fileName: script.fileName, source: script.code },
    { fileName: OFFLINE_PAGE.slice(1), source: page },
    { fileName: "sw.js", source: worker },
  ];
}

/** The paths the worker leaves alone are the dev server's proxy keys: the one
 *  list of what the api owns on this origin (docs/explanation/pwa.md). */
export function pwa(options: Readonly<{ release: string }>): Plugin {
  let passThrough: readonly string[] = [];
  return {
    name: "margince-pwa",
    apply: "build",
    // After Vite's own HTML plugin, whose index.html is how this knows it is the SPA.
    enforce: "post",
    configResolved(config) {
      passThrough = Object.keys(config.server.proxy ?? {});
    },
    async generateBundle(_output, bundle) {
      // Storybook builds through this config too, and its bundle has no app to serve.
      if (!("index.html" in bundle)) {
        return;
      }
      if (passThrough.length === 0) {
        throw new Error(
          "vite.config.ts proxies nothing, so the service worker would take every api navigation for the app's",
        );
      }
      for (const file of await buildPwaFiles({
        release: options.release,
        passThrough,
      })) {
        this.emitFile({
          type: "asset",
          fileName: file.fileName,
          source: file.source,
        });
      }
    },
  };
}
