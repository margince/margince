// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, srcDir } from "../tree";
import { namesServiceWorker, rule } from "./serviceworker-registrar.census";

describe("the serviceworker-registrar census", () => {
  it.each([
    ["aliased.js", "const container = navigator.serviceWorker;"],
    ["destructured.mjs", "const { serviceWorker: sw } = navigator;"],
    ["indexed.jsx", 'navigator["serviceWorker"].register("/x.js");'],
    ["templated.ts", "navigator[`serviceWorker`].register('/x.js');"],
  ])("sees the container reached for in %s", (file, text) => {
    expect(namesServiceWorker(parseSource(file, text))).toBe(true);
  });

  it("does not read a comment as a registration", () => {
    const text = "// navigator.serviceWorker.register('/x.js');\nexport {};";
    expect(namesServiceWorker(parseSource("comment.ts", text))).toBe(false);
  });
});

describe("the serviceworker-registrar rule", () => {
  it("reports a second registrar, and leaves its test alone", () => {
    const text = "navigator.serviceWorker.register('/x.js');\n";
    expect(
      plantedFindings(rule, [
        { path: join(srcDir, "lib", "pwa.ts"), area: "src", text },
        { path: join(srcDir, "lib", "planted.ts"), area: "src", text },
        { path: join(srcDir, "lib", "planted.test.ts"), area: "src", text },
      ]),
    ).toEqual(["src/lib/planted.ts names navigator.serviceWorker"]);
  });
});
