// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { plantedFindings, repoRoot, srcDir } from "../tree";
import {
  EXEMPT,
  OWNER,
  operatorLiterals,
  rule,
  shipped,
} from "./filtersentence.census";

const literalsIn = (text: string) =>
  operatorLiterals(parseSource("planted.tsx", text));

describe("the filtersentence rule", () => {
  it("reports a planted writer in each shape it reads", () => {
    const planted = [
      `const label = t("filters.op.eq");`,
      // biome-ignore lint/suspicious/noTemplateCurlyInString: a fixture of source code
      "const label = t(`filters.op.${op}`);",
      "const KEYS = { gt: `filters.op.moreThan` };",
      `const label = t("filters.op" + "." + op);`,
      // biome-ignore lint/suspicious/noTemplateCurlyInString: a fixture of source code
      "const label = t(`filters.op${dot}${op}`);",
    ].join("\n");
    expect(literalsIn(planted)).toEqual([
      "filters.op.eq",
      "filters.op.",
      "filters.op.moreThan",
      "filters.op",
      "filters.op",
    ]);
    const neighbours = [
      `// t("filters.op.eq")`,
      `t("filters.operator");`,
      // biome-ignore lint/suspicious/noTemplateCurlyInString: a fixture of source code
      "t(`filters.operator${suffix}`);",
      `t("filters.opened");`,
    ].join("\n");
    expect(literalsIn(neighbours)).toEqual([]);
  });

  it("leaves tests, stories, kits and the catalogs out of the census", () => {
    expect(shipped("frontend/src/features/lists/filterpage.test.tsx")).toBe(
      false,
    );
    expect(shipped("frontend/src/features/lists/filters.stories.tsx")).toBe(
      false,
    );
    expect(shipped("frontend/src/features/lists/filters.testkit.ts")).toBe(
      false,
    );
    expect(shipped("frontend/src/i18n/en.ts")).toBe(false);
    expect(shipped("frontend/src/features/lists/listexplain.tsx")).toBe(true);
    expect(shipped("extensions/openchannel/frontend/screen.tsx")).toBe(true);
  });

  it("fails a planted writer and a stale exemption, and spares the owner", () => {
    const text = 'const label = t("filters.op.eq");\n';
    const findings = plantedFindings(rule, [
      { path: join(repoRoot, OWNER), area: "src", text },
      { path: join(srcDir, "features", "x", "picker.tsx"), area: "src", text },
      {
        path: join(srcDir, "features", "x", "picker.test.tsx"),
        area: "src",
        text,
      },
    ]);
    expect(findings).toEqual([
      "frontend/src/features/x/picker.tsx: spells a filters.op. message",
      ...[...EXEMPT].map(
        ([path, why]) =>
          `${path}: no longer spells a filters.op. message, so its exemption (${why}) is stale and must go`,
      ),
    ]);
  });
});
