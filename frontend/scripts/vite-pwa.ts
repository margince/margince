// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import { build, type Plugin, type Rolldown, runnerImport } from "vite";

const FRONTEND = resolve(dirname(fileURLToPath(import.meta.url)), "..");
const WORKER_SOURCE = resolve(FRONTEND, "src/offline/serviceworker.js");

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

function digest(...parts: readonly string[]): string {
  const hash = createHash("sha256");
  for (const part of parts) {
    hash.update(part).update("\0");
  }
  return hash.digest("hex").slice(0, 12);
}

/** Keyed on the release and on what the worker holds, so each change gives the
 *  worker new bytes and every browser installs it afresh. */
export function workerCacheName(
  parts: Readonly<{
    release: string;
    page: string;
    workerSource: string;
    passThrough: readonly string[];
  }>,
): string {
  const held = digest(parts.page, parts.workerSource, ...parts.passThrough);
  return `margince-offline-${parts.release || "dev"}-${held}`;
}

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
  // Under /assets/, where a replica of an older deploy answers 404 rather than
  // the app shell, so the install fails instead of keeping the shell as this page.
  const pageFile = `assets/offline-${digest(page).slice(0, 8)}.html`;
  const worker = renderServiceWorker({
    cacheName: workerCacheName({
      release: options.release,
      page,
      workerSource: readFileSync(WORKER_SOURCE, "utf8"),
      passThrough: options.passThrough,
    }),
    offlinePage: `/${pageFile}`,
    offlineScript,
    passThrough: options.passThrough,
  });
  return [
    { fileName: script.fileName, source: script.code },
    { fileName: pageFile, source: page },
    { fileName: "sw.js", source: worker },
  ];
}

/** The worker leaves alone every path under a key the dev server proxies to the api,
 *  by whole segment; frontend/vite-proxy.test.ts holds the launcher's list to the keys. */
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
