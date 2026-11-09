// The web side of the view digest pin: the manifest names every document it
// was given, and each entry is the SHA-256 of that document's bytes on disk.

import { createHash } from "node:crypto";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, describe, expect, it } from "vitest";
import { DIGESTS_FILE, writeDigests } from "./mcp-app-digests.mjs";

/** @type {string[]} */
const made = [];

afterEach(() => {
  for (const dir of made.splice(0)) {
    rmSync(dir, { recursive: true, force: true });
  }
});

function builtViews() {
  const dir = mkdtempSync(join(tmpdir(), "mcp-app-digests-"));
  made.push(dir);
  writeFileSync(
    join(dir, "approval.html"),
    "<!doctype html><title>Approval</title>\n",
  );
  writeFileSync(
    join(dir, "update.html"),
    "<!doctype html><title>Update</title>é\n",
  );
  return dir;
}

/** @param {string} file */
function sha256Of(file) {
  return createHash("sha256").update(readFileSync(file)).digest("hex");
}

describe("writeDigests", () => {
  it("writes one entry per document, each the SHA-256 of its bytes", async () => {
    const dir = builtViews();
    await writeDigests(dir, ["update.html", "approval.html"]);
    const manifest = JSON.parse(readFileSync(join(dir, DIGESTS_FILE), "utf8"));
    expect(Object.keys(manifest).sort()).toEqual([
      "approval.html",
      "update.html",
    ]);
    for (const file of ["approval.html", "update.html"]) {
      expect(manifest[file]).toBe(sha256Of(join(dir, file)));
    }
  });

  it("writes the manifest as a single line the api build can link", async () => {
    const dir = builtViews();
    await writeDigests(dir, ["approval.html", "update.html"]);
    const written = readFileSync(join(dir, DIGESTS_FILE), "utf8");
    expect(written.trimEnd()).not.toContain("\n");
    expect(written).not.toContain("'");
  });
});
