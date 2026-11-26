import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import {
  type APIRequestContext,
  type APIResponse,
  expect,
  test,
} from "@playwright/test";
import { parse } from "node-html-parser";

// What the built app serves at its manifest link, read from the preview server
// over HTTP. A file that parses on disk proves nothing about the answer a
// browser receives, which is where "Line: 1, column: 1, Syntax error" comes from.

type Delivery = "manifest" | "not-modified" | "fallback-html" | "unreadable";

// The name the shipped manifest carries, read from the file the build copies.
const shippedName: string = JSON.parse(
  readFileSync(
    join(
      dirname(fileURLToPath(import.meta.url)),
      "..",
      "public",
      "manifest.webmanifest",
    ),
    "utf8",
  ),
).name;

// classify names what a manifest request actually got. A 304 carries no body
// and is a valid revalidation. Anything else must be JSON naming this app.
async function classify(response: APIResponse): Promise<Delivery> {
  if (response.status() === 304) return "not-modified";
  const type = response.headers()["content-type"] ?? "";
  if (type.includes("text/html")) return "fallback-html";
  try {
    const body: unknown = JSON.parse(await response.text());
    const name =
      typeof body === "object" && body !== null && "name" in body
        ? body.name
        : undefined;
    return name === shippedName ? "manifest" : "unreadable";
  } catch {
    return "unreadable";
  }
}

// The href the served index.html names, read from its parsed head. The test
// follows the page's own link, never a match inside a comment.
async function manifestHref(request: APIRequestContext): Promise<string> {
  const page = parse(await (await request.get("/")).text());
  const href = page
    .querySelector('head link[rel~="manifest"]')
    ?.getAttribute("href");
  expect(href, "the served index.html names no manifest").toBeTruthy();
  return href as string;
}

test("a fresh manifest request answers JSON naming the app", async ({
  request,
}) => {
  const response = await request.get(await manifestHref(request));
  expect(response.status()).toBe(200);
  expect(await classify(response)).toBe("manifest");
});

test("a revalidation answers 304 with no body, told apart from a fresh one", async ({
  request,
}) => {
  const href = await manifestHref(request);
  const fresh = await request.get(href);
  const etag = fresh.headers().etag;
  const modified = fresh.headers()["last-modified"];
  expect(etag ?? modified, "the manifest carries no validator").toBeTruthy();

  const again = await request.get(href, {
    headers: etag
      ? { "If-None-Match": etag }
      : { "If-Modified-Since": modified },
  });
  expect(await classify(again)).toBe("not-modified");
  expect((await again.body()).length).toBe(0);
  expect(await classify(fresh)).toBe("manifest");
});

test("a manifest name no file answers is classified as fallback HTML, never as a manifest", async ({
  request,
}) => {
  // The SPA answers any path no file holds with index.html. A wrong href then
  // reaches the browser as a page, which it reports as an invalid manifest.
  const response = await request.get("/no-such-file.webmanifest");
  expect(await classify(response)).toBe("fallback-html");
});
