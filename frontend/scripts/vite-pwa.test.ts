// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { createContext, runInContext } from "node:vm";
import { build, type InlineConfig } from "vite";
import { beforeAll, describe, expect, it, vi } from "vitest";
import { SERVICE_WORKER_URL } from "../src/app/pwa";
import { LOCALES } from "../src/i18n";
import spaConfig from "../vite.config";
import { pwa, workerCacheName } from "./vite-pwa";

const ORIGIN = "https://margince.test";
const RELEASE = "2026.9.1";
const WORKER_SOURCE = join(
  dirname(fileURLToPath(import.meta.url)),
  "../src/offline/serviceworker.js",
);

/** A plugin option is a plugin, a nested array of them, a promise, or nothing. */
function pluginNames(options: unknown): string[] {
  if (Array.isArray(options)) return options.flatMap(pluginNames);
  if (typeof options !== "object" || options === null) return [];
  return "name" in options && typeof options.name === "string"
    ? [options.name]
    : [];
}

// A site with nothing in it but a document, built through the plugin the way
// the SPA's own build runs it, with the SPA's own proxy list.
async function buildSite(document: string): Promise<Map<string, string>> {
  const root = mkdtempSync(join(tmpdir(), "margince-pwa-"));
  try {
    writeFileSync(join(root, document), "<!doctype html><title>t</title>");
    const config: InlineConfig = {
      configFile: false,
      root,
      logLevel: "warn",
      plugins: [pwa({ release: RELEASE })],
      server: { proxy: spaConfig.server?.proxy },
      build: {
        write: false,
        rolldownOptions: { input: join(root, document) },
      },
    };
    const result = await build(config);
    const outputs = (Array.isArray(result) ? result : [result]).flatMap(
      (bundle) => ("output" in bundle ? bundle.output : []),
    );
    return new Map(
      outputs.map((item) => [
        item.fileName,
        item.type === "chunk" ? item.code : String(item.source),
      ]),
    );
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

type Settled = Readonly<{ waitUntil: (work: Promise<unknown>) => void }>;
type Navigation = Readonly<{ url: string; mode: string }>;
type Handler = (event: object) => void;

/**
 * Loads the emitted worker into a stand-in for its global scope, as the one
 * settings line it starts with and then the source file it ends with.
 */
function startWorker(emitted: string, network: (url: string) => Response) {
  const source = readFileSync(WORKER_SOURCE, "utf8");
  const settings = emitted.slice(0, emitted.length - source.length);
  expect(emitted.endsWith(source)).toBe(true);
  expect(settings).toMatch(/^self\.__MARGINCE_SW_SETTINGS__ = \{[^\n]*\};\n$/);
  const handlers = new Map<string, Handler>();
  const stores = new Map<string, Map<string, Response>>();
  const urlOf = (input: string | Navigation) =>
    new URL(typeof input === "string" ? input : input.url, ORIGIN).href;
  const fetch = vi.fn(async (input: string | Navigation) =>
    network(urlOf(input)),
  );
  const store = (name: string) => {
    const entries = stores.get(name) ?? new Map<string, Response>();
    stores.set(name, entries);
    return entries;
  };
  const scope = {
    location: { origin: ORIGIN },
    addEventListener: (type: string, handler: Handler) => {
      handlers.set(type, handler);
    },
    caches: {
      open: async (name: string) => ({
        addAll: async (requests: readonly Navigation[]) => {
          const responses = await Promise.all(requests.map(fetch));
          for (const [index, response] of responses.entries()) {
            if (!response.ok) {
              throw new TypeError(`${requests[index]?.url} ${response.status}`);
            }
            store(name).set(requests[index]?.url ?? "", response);
          }
        },
      }),
      keys: async () => [...stores.keys()],
      delete: async (name: string) => stores.delete(name),
      match: async (url: string, options: Readonly<{ cacheName: string }>) =>
        stores.get(options.cacheName)?.get(url)?.clone(),
    },
    clients: { claim: vi.fn(async () => undefined) },
    registration: {
      navigationPreload: { enable: vi.fn(async () => undefined) },
    },
    skipWaiting: vi.fn(async () => undefined),
    fetch,
  };
  const context = createContext({ self: scope, URL, Request, Response });
  runInContext(settings, context);
  runInContext(source, context, {
    filename: pathToFileURL(WORKER_SOURCE).href,
  });

  async function lifecycle(type: "install" | "activate"): Promise<void> {
    const work: Promise<unknown>[] = [];
    const event: Settled = { waitUntil: (promise) => work.push(promise) };
    handlers.get(type)?.(event);
    await Promise.all(work);
  }

  /** What the worker answered with, or undefined when it left the request alone. */
  function request(url: string, mode: string): Promise<Response> | undefined {
    let answer: Promise<Response> | undefined;
    handlers.get("fetch")?.({
      request: { url: new URL(url, ORIGIN).href, mode },
      respondWith: (response: Promise<Response>) => {
        answer = response;
      },
      waitUntil: () => undefined,
    });
    return answer;
  }

  return { scope, stores, lifecycle, request };
}

let site: Map<string, string>;

/** The offline page's own name, content-hashed under assets/. */
function pageFile(): string {
  const [name, ...others] = [...site.keys()].filter((file) =>
    /^assets\/offline-[0-9a-f]{8}\.html$/.test(file),
  );
  if (name === undefined || others.length > 0) {
    throw new Error(
      `the build emitted no one offline page: ${[...site.keys()].join(", ")}`,
    );
  }
  return name;
}

function settingsOf(
  worker: string,
): Readonly<{ cacheName?: unknown; offlinePage?: unknown }> {
  const line = /^self\.__MARGINCE_SW_SETTINGS__ = (\{[^\n]*\});\n/.exec(
    worker,
  )?.[1];
  const parsed: unknown = JSON.parse(line ?? "null");
  return typeof parsed === "object" && parsed !== null ? parsed : {};
}

function emitted(fileName: string): string {
  const source = site.get(fileName);
  if (source === undefined) {
    throw new Error(
      `the build emitted no ${fileName}: ${[...site.keys()].join(", ")}`,
    );
  }
  return source;
}

beforeAll(async () => {
  site = await buildSite("index.html");
}, 60_000);

describe("the build", () => {
  it("runs in the SPA's own build", () => {
    expect(pluginNames(spaConfig.plugins)).toContain("margince-pwa");
  });

  it("emits the worker the app registers at the site root, and its page under assets/", () => {
    expect(site.has(SERVICE_WORKER_URL.slice(1))).toBe(true);
    expect(settingsOf(emitted("sw.js")).offlinePage).toBe(`/${pageFile()}`);
    expect(site.has("offline.html")).toBe(false);
  });

  it("names the worker's cache for the release and for exactly what it holds", () => {
    expect(settingsOf(emitted("sw.js")).cacheName).toBe(
      workerCacheName({
        release: RELEASE,
        page: emitted(pageFile()),
        workerSource: readFileSync(WORKER_SOURCE, "utf8"),
        passThrough: Object.keys(spaConfig.server?.proxy ?? {}),
      }),
    );
  });

  it("builds the same worker twice from the same tree", async () => {
    const again = await buildSite("index.html");
    expect(again.get("sw.js")).toBe(emitted("sw.js"));
  }, 60_000);

  it("renames the cache when the page, the worker, the api's paths or the release change", () => {
    const parts = {
      release: RELEASE,
      page: "<main>page</main>",
      workerSource: "self.addEventListener();",
      passThrough: ["/v1", "/oauth"],
    };
    const name = workerCacheName(parts);
    expect(workerCacheName({ ...parts })).toBe(name);
    for (const changed of [
      { ...parts, page: "<main>page!</main>" },
      { ...parts, workerSource: "self.addEventListener(); " },
      { ...parts, passThrough: ["/v1"] },
      { ...parts, passThrough: ["/v1/oauth"] },
      { ...parts, release: "2026.9.2" },
    ]) {
      expect(workerCacheName(changed)).not.toBe(name);
    }
  });

  it("emits nothing into a build that is not the SPA's", async () => {
    const storybook = await buildSite("iframe.html");
    expect([...storybook.keys()]).not.toContain("sw.js");
  }, 60_000);
});

describe("the offline page", () => {
  it("speaks every locale the app ships, the first one shown", () => {
    const page = emitted(pageFile());
    const blocks = [
      ...page.matchAll(/<main class="offline" lang="(\w+)"( hidden)?>/g),
    ];
    expect(blocks.map(([, locale]) => locale)).toEqual([...LOCALES]);
    expect(blocks.map(([, , hidden]) => hidden === undefined)).toEqual([
      true,
      false,
      false,
    ]);
  });

  it("carries its styles and loads its one script by address, never inline", () => {
    const page = emitted(pageFile());
    const scripts = [...page.matchAll(/<script\b([^>]*)>/g)].map(
      ([, attributes]) => attributes,
    );
    expect(scripts).toHaveLength(1);
    const src = /\bsrc="\/([^"]+)"/.exec(scripts[0] ?? "")?.[1];
    expect(src !== undefined && site.has(src)).toBe(true);
    expect(page).toMatch(/<style>[^<]*--bgPage/);
    expect(page).not.toMatch(/<link\b/);
  });
});

describe("the service worker", () => {
  const page = () => emitted(pageFile());
  const script = () => /src="(\/assets\/[^"]+)"/.exec(page())?.[1] ?? "";
  const online = (url: string) => {
    if (url === `${ORIGIN}/${pageFile()}`) {
      return new Response(page(), { headers: { "content-type": "text/html" } });
    }
    return url === `${ORIGIN}${script()}`
      ? new Response(emitted(script().slice(1)))
      : new Response("app");
  };

  async function installed(network: (url: string) => Response = online) {
    const worker = startWorker(emitted("sw.js"), network);
    await worker.lifecycle("install");
    return worker;
  }

  it("precaches the offline page and its script, and takes over at once", async () => {
    const worker = await installed();
    const [cache] = [...worker.stores.values()];
    expect([...(cache?.keys() ?? [])]).toEqual([
      `${ORIGIN}/${pageFile()}`,
      `${ORIGIN}${script()}`,
    ]);
    expect(worker.scope.skipWaiting).toHaveBeenCalled();
  });

  it("does not install without the offline page's script", async () => {
    const worker = startWorker(emitted("sw.js"), (url) =>
      url === `${ORIGIN}${script()}`
        ? new Response("", { status: 404 })
        : online(url),
    );
    await expect(worker.lifecycle("install")).rejects.toThrow(TypeError);
    expect(worker.scope.skipWaiting).not.toHaveBeenCalled();
  });

  it("answers the offline page's script from its cache, with no network at all", async () => {
    const worker = await installed();
    worker.scope.fetch.mockRejectedValue(new TypeError("Failed to fetch"));
    const answer = await worker.request(script(), "cors");
    expect(await answer?.text()).toBe(emitted(script().slice(1)));
  });

  it("fetches the offline page's script when its cache has lost it", async () => {
    const worker = await installed();
    worker.stores.clear();
    const answer = await worker.request(script(), "cors");
    expect(await answer?.text()).toBe(emitted(script().slice(1)));
  });

  it("leaves every other built asset to the browser", async () => {
    const worker = await installed();
    const others = [...site.keys()].filter(
      (name) => name.startsWith("assets/") && `/${name}` !== script(),
    );
    for (const name of [
      ...others,
      "assets/index-Bx1a2b3c.js",
      "assets/offline-Zz9y8x7w.js",
    ]) {
      expect(worker.request(`/${name}`, "cors"), name).toBeUndefined();
    }
    expect(worker.request(`${script()}?v=1`, "cors")).toBeUndefined();
  });

  it("leaves every other request that is not a navigation to the browser", async () => {
    const worker = await installed();
    for (const mode of ["cors", "no-cors", "same-origin"]) {
      expect(worker.request("/assets/index.js", mode)).toBeUndefined();
      expect(worker.request("/", mode)).toBeUndefined();
    }
  });

  it("leaves a navigation into the api's paths to the browser", async () => {
    const worker = await installed();
    for (const url of [
      "/v1/me",
      "/oauth/authorize?client_id=x",
      "/.well-known/oauth-authorization-server",
      "/mcp",
      "/mcp/sse",
      "/setup",
      "/webhooks/in",
      "/healthz",
      "/readyz",
      "/metrics",
    ]) {
      expect(worker.request(url, "navigate"), url).toBeUndefined();
    }
  });

  it("answers a navigation with the network's response, a server error included", async () => {
    const failing = new Response("broken", { status: 500 });
    const worker = await installed((url) =>
      url === `${ORIGIN}/` ? failing : online(url),
    );
    expect(await worker.request("/", "navigate")).toBe(failing);
  });

  it("never turns on navigation preload, which would fetch a pass-through navigation twice", async () => {
    const worker = await installed();
    await worker.lifecycle("activate");
    expect(
      worker.scope.registration.navigationPreload.enable,
    ).not.toHaveBeenCalled();
    worker.scope.fetch.mockClear();
    expect(
      worker.request("/v1/public/confirm/token", "navigate"),
    ).toBeUndefined();
    expect(worker.scope.fetch).not.toHaveBeenCalled();
  });

  it("answers a navigation the network could not make with the offline page", async () => {
    const worker = await installed();
    worker.scope.fetch.mockRejectedValue(new TypeError("Failed to fetch"));
    const answer = await worker.request("/", "navigate");
    expect(await answer?.text()).toBe(page());
  });

  it("gives the offline page to a failed navigation that only starts like an api path, a shipped /mcp-apps view among them", async () => {
    const worker = await installed();
    worker.scope.fetch.mockRejectedValue(new TypeError("Failed to fetch"));
    for (const url of [
      "/mcp-apps/create-followups.html",
      "/v1beta",
      "/setup-guide",
    ]) {
      const answer = await worker.request(url, "navigate");
      expect(await answer?.text(), url).toBe(page());
    }
  });

  it("leaves the failure to the browser when its own cache is gone", async () => {
    const worker = await installed();
    worker.stores.clear();
    worker.scope.fetch.mockRejectedValue(new TypeError("Failed to fetch"));
    const answer = await worker.request("/", "navigate");
    expect(answer?.type).toBe("error");
  });

  it("deletes every cache but its own when it activates", async () => {
    const worker = await installed();
    worker.stores.set("margince-shell-v1", new Map());
    worker.stores.set("margince-offline-2026.8.0-000000000000", new Map());
    const [own] = [...worker.stores.keys()];
    await worker.lifecycle("activate");
    expect([...worker.stores.keys()]).toEqual([own]);
    expect(worker.scope.clients.claim).toHaveBeenCalled();
  });
});
