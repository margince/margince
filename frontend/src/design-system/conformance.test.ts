import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const frontendRoot = join(import.meta.dirname, "..", "..");

describe("the web-app manifest", () => {
  it("is valid and complete for installability", () => {
    const manifest = JSON.parse(
      readFileSync(
        join(frontendRoot, "public", "manifest.webmanifest"),
        "utf8",
      ),
    );
    expect(manifest.name).toBe("Margince");
    expect(manifest.start_url).toBe("/");
    expect(manifest.display).toBe("standalone");
    expect(manifest.icons.length).toBeGreaterThanOrEqual(1);
  });
});
