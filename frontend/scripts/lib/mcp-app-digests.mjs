// The digest manifest of the built MCP App view documents. The api is linked
// with this file and serves a view only when its bytes hash to the entry here.

import { createHash } from "node:crypto";
import { readFile, writeFile } from "node:fs/promises";
import { stable } from "./stable.mjs";

/** The manifest's file name beside the documents. */
export const DIGESTS_FILE = "digests.json";

/**
 * Hashes each document in outDir and writes the manifest beside them, as one
 * line of JSON: file name to the hex SHA-256 of its final bytes.
 *
 * @param {string} outDir
 * @param {readonly string[]} files
 * @returns {Promise<Record<string, string>>}
 */
export async function writeDigests(outDir, files) {
  /** @type {Record<string, string>} */
  const manifest = {};
  for (const file of [...files].sort(stable)) {
    const bytes = await readFile(`${outDir}/${file}`);
    manifest[file] = createHash("sha256").update(bytes).digest("hex");
  }
  await writeFile(`${outDir}/${DIGESTS_FILE}`, `${JSON.stringify(manifest)}\n`);
  return manifest;
}
