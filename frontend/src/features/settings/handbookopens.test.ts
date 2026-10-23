// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { readdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { SETTINGS_PAGES } from "../../lib/settingscatalog";

// A handbook section names its settings page with an
// `<!-- opens: settings/<id> -->` marker, and the search box turns that id into
// a button. An id the catalog does not hold is a button to nowhere. So every
// marker in the handbook must name a page this table lists.

const handbook = join(
  dirname(fileURLToPath(import.meta.url)),
  "..",
  "..",
  "..",
  "..",
  "docs",
  "handbook",
);

function markers(): { page: string; id: string }[] {
  return readdirSync(handbook)
    .filter((name) => name.endsWith(".md"))
    .flatMap((page) =>
      [
        ...readFileSync(join(handbook, page), "utf8").matchAll(
          /<!-- opens: settings\/([^ ]+) -->/g,
        ),
      ].map((found) => ({ page, id: found[1] ?? "" })),
    );
}

describe("handbook opens markers", () => {
  it("name only settings pages the catalog lists", () => {
    const found = markers();
    // An empty census is a failure: a wrong path reads nothing and passes.
    expect(found.length).toBeGreaterThan(0);
    const known = new Set<string>(SETTINGS_PAGES.map((page) => page.id));
    const unknown = found.filter((marker) => !known.has(marker.id));
    expect(unknown).toEqual([]);
  });
});
