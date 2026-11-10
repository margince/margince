// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir, treeOf } from "../tree";
import { builtAddresses, rule } from "./vite-proxy.census";

const at = (name: string, text: string) => ({
  path: join(srcDir, "features", name),
  area: "src" as const,
  text,
});

const read = (...planted: ReturnType<typeof at>[]) => {
  const tree = treeOf(planted);
  return builtAddresses(rule.corpus(tree), tree);
};

describe("the vite-proxy census", () => {
  it("reads only the root interpolation of a template", () => {
    const { built } = read(
      at("a.ts", `const u = \`\${location.origin}/v1/offers/\${id}/pdf\`;`),
    );
    expect([...built.keys()]).toEqual(["v1"]);
  });

  it("follows an origin into the module that builds the path", () => {
    const { built, byNeighbourhood } = read(
      at(
        "screen.tsx",
        'import { inboundUrl } from "./contract";\nconst base = globalThis.location.origin;\n',
      ),
      at(
        "contract.ts",
        `export const inboundUrl = (b) => \`\${b}/webhooks/in\`;`,
      ),
    );
    expect(built.get("webhooks")).toBe("src/features/contract.ts");
    expect([...byNeighbourhood]).toEqual(["webhooks"]);
  });
});

describe("the vite-proxy rule", () => {
  it("reports an address at a prefix the dev server does not proxy", () => {
    expect(
      plantedFindings(rule, [
        at("a.ts", `const u = \`\${origin}/planted-edge/x\`;`),
      ]),
    ).toEqual([
      "src/features/a.ts builds an address at /planted-edge from the app's own origin, which the dev server does not proxy",
    ]);
  });
});
