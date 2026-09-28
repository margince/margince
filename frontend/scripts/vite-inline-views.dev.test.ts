import { createServer, type Plugin, type ViteDevServer } from "vite";
import { afterAll, beforeAll, expect, it } from "vitest";
import spaConfig from "../vite.config";
import {
  inspectDocument,
  serveMcpApps,
  validateDocument,
} from "./vite-inline-views";

let server: ViteDevServer;
/** Where the server this spec started is actually listening. */
let base: string;

// THE OS CHOOSES THE PORT, and the port is then read back off the server.
//
// It was 5199 with `strictPort`, which made `make check-fe` unable to run twice
// on one machine: the second run died with "Port 5199 is already in use" and
// reported a red naming a file the reader had not touched, in a suite about
// inline views. This tree is worked in linked worktrees — `make dev` gives each
// one its own database, Redis logical database, port pair and bucket with no
// flag to remember — and the frontend gate was the one place that stopped being
// true.
//
// Port 0 rather than a port derived per worktree: what this spec asserts is
// what the dev server SERVES, never where, so the right fix removes the shared
// resource instead of partitioning it. A second hard-coded number would answer
// "the port is not 5199" and leave two concurrent runs still colliding.
beforeAll(async () => {
  server = await createServer({
    configFile: false,
    plugins: [serveMcpApps()],
    server: { port: 0 },
    // Nothing here serves the SPA, so crawling its entry to pre-bundle
    // dependencies is work this spec neither needs nor can do without the SPA's
    // own plugins.
    optimizeDeps: { noDiscovery: true },
    logLevel: "error",
  });
  await server.listen();
  base = servedAt(server);
});

/** The origin a server is listening on, read off the server itself. */
function servedAt(started: ViteDevServer): string {
  const address = started.httpServer?.address();
  if (
    address === null ||
    address === undefined ||
    typeof address === "string"
  ) {
    // A string address is a unix socket, which this spec cannot fetch from, and
    // a missing one means listen() resolved without binding. Both are the
    // harness failing rather than the subject, so they say so here instead of
    // surfacing as a fetch to `http://undefined`.
    throw new Error(
      `the dev server is not listening on a TCP port: ${String(address)}`,
    );
  }
  return `http://localhost:${address.port}`;
}

afterAll(async () => {
  await server.close();
});

it("serves a document the admission check would accept", async () => {
  const res = await fetch(`${base}/mcp-apps/company-brief.html`);
  expect(res.status).toBe(200);
  expect(res.headers.get("content-type")).toMatch(/text\/html/);
  const doc = await res.text();
  expect(validateDocument(doc)).toEqual([]);
  expect(inspectDocument(doc)).toEqual([]);
  expect(doc).toContain("<title>Morning brief</title>");
  expect(doc).toContain("SPDX-License-Identifier: BUSL-1.1");
}, 30_000);

it("404s a view that does not exist rather than answering with something else", async () => {
  // The production posture, mirrored: nginx's try_files ... =404 exists because
  // a fallback answering 200 would hand the api an app shell it would then
  // believe was a view.
  const res = await fetch(`${base}/mcp-apps/nope.html`);
  expect(res.status).toBe(404);
}, 30_000);

it("is wired into the dev server `make dev` actually starts", () => {
  // Without this the middleware above is a facility nothing reaches: `make dev`
  // runs vite.config.ts, not the mcp-apps build config, so a request for a view
  // would fall through the SPA fallback to a dev index.html carrying `src=`
  // module scripts and /@vite/client — both refused by name, leaving both views
  // permanently unadvertised in every dev stack.
  expect(pluginNames(spaConfig.plugins)).toContain("mcp-apps:serve-views");
});

// TWO AT ONCE, which is the property that was actually broken.
//
// "The port is not 5199" would be satisfied by a second hard-coded number and
// would leave concurrent runs colliding exactly as before. What has to hold is
// that two of these servers can stand at the same time — which is what two
// `make check-fe` runs on one machine amount to — so that is what is asserted.
// This case fails on the old configuration and passes on this one.
it("lets a second server stand beside the first", async () => {
  const second = await createServer({
    configFile: false,
    plugins: [serveMcpApps()],
    server: { port: 0 },
    optimizeDeps: { noDiscovery: true },
    logLevel: "error",
  });
  try {
    await second.listen();
    const other = servedAt(second);
    expect(other).not.toBe(base);
    const res = await fetch(`${other}/mcp-apps/company-brief.html`);
    expect(res.status).toBe(200);
  } finally {
    await second.close();
  }
}, 30_000);

/** A plugin option is a plugin, a nested array of them, a promise, or nothing —
 *  so the names are collected by walking rather than by flattening, which TypeScript
 *  cannot type for an arbitrarily nested recursive union. */
function pluginNames(options: unknown): string[] {
  if (Array.isArray(options)) return options.flatMap(pluginNames);
  if (typeof options !== "object" || options === null) return [];
  const named = options as Partial<Plugin>;
  return typeof named.name === "string" ? [named.name] : [];
}
