// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, scriptsDir, srcDir } from "../tree";
import { rule } from "./one-parser.census";

const call = (name: string) => `${["ts", name].join(".")}("a.ts", "", 99);\n`;

describe("the one-parser rule", () => {
  it("fails a second parse site and passes the owner", () => {
    expect(
      plantedFindings(rule, [
        {
          path: join(scriptsDir, "lib", "source-tree.ts"),
          area: "scripts",
          text: call("createSourceFile"),
        },
        {
          path: join(srcDir, "features", "planted.ts"),
          area: "src",
          text: call("createProgram"),
        },
        { path: join(srcDir, "features", "clean.ts"), area: "src", text: "" },
      ]),
    ).toEqual(["src/features/planted.ts calls the compiler's parser"]);
  });
});
