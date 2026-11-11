import { type APIResponse, expect, test } from "@playwright/test";

// What the built app serves at its manifest link, read from the preview server
// over HTTP. A file that parses on disk proves nothing about the answer a
// browser receives, which is where "Line: 1, column: 1, Syntax error" comes from.

type Delivery = "manifest" | "not-modified" | "fallback-html" | "unreadable";

// classify names what a manifest request actually got. A 304 carries no body
// and is a valid revalidation; anything else must be JSON naming the app.
async function classify(response: APIResponse): Promise<Delivery> {
  if (response.status() === 304) return "not-modified";
  const type = response.headers()["content-type"] ?? "";
  if (type.includes("text/html")) return "fallback-html";
  try {
    const body: unknown = JSON.parse(await response.text());
    const named = typeof body === "object" && body !== null && "name" in body;
    return named ? "manifest" : "unreadable";
  } catch {
    return "unreadable";
  }
}

// The href the served index.html names, so the test follows the page's own link.
async function manifestHref(
  request: import("@playwright/test").APIRequestContext,
) {
  const page = await (await request.get("/")).text();
  const href = /<link[^>]+rel="manifest"[^>]+href="([^"]+)"/.exec(page)?.[1];
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
