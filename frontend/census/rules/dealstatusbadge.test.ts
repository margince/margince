// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { parseSource } from "../../scripts/lib/source-tree";
import { ruleFindings, srcDir, treeOf } from "../tree";
import { rawStatusRenders, rule } from "./dealstatusbadge.census";

describe("a record's status is never drawn as its stored code", () => {
  it("finds the raw form in a planted screen, and passes the badge", () => {
    expect(
      rawStatusRenders(
        parseSource(
          "/app/src/planted.tsx",
          [
            `const a = <Badge>{deal.status}</Badge>;`,
            `const b = <Badge>{row.status}</Badge>;`,
            `const c = <span>{offer?.status ?? "-"}</span>;`,
            `const d = <span>{ok ? label : (item.status)}</span>;`,
            `const e = <DealStatusBadge status={deal.status} />;`,
            `const f = <span>{row.statusLabel}</span>;`,
            `const g = <span>{key ? t(key) : row.status}</span>;`,
            `const h = <span>{row.status as string}</span>;`,
            `const i = <span>{row.status!}</span>;`,
            `const j = <span>{\`\${row.status}\`}</span>;`,
            `const k = <span>{String(row.status)}</span>;`,
            `const l = <span>{ok ? t("deal.status.open") : row.status}</span>;`,
            `const m = <span>{(row.status satisfies string)}</span>;`,
            `const n = <span>{shown && row.status}</span>;`,
            `const o = <span>{row.status.replace("_", " ")}</span>;`,
            `const p = <span>{row.status?.toUpperCase()}</span>;`,
            `const q = <span>{row.status === "won" && <b>Won</b>}</span>;`,
            `const r = <span>{statusLabel(row.status)}</span>;`,
          ].join("\n"),
        ),
      ),
    ).toEqual([
      "{deal.status}",
      "{row.status}",
      '{offer?.status ?? "-"}',
      "{ok ? label : (item.status)}",
      "{row.status as string}",
      "{row.status!}",
      `{\`\${row.status}\`}`,
      "{String(row.status)}",
      '{ok ? t("deal.status.open") : row.status}',
      "{(row.status satisfies string)}",
      "{shown && row.status}",
      '{row.status.replace("_", " ")}',
      "{row.status?.toUpperCase()}",
    ]);
  });

  // The corpus is what the real bundle entries reach, and a planted tree
  // holds none of that. So the planted screen is handed to the rule directly.
  it("reports a planted raw status through the rule's own visit", () => {
    const tree = treeOf([
      {
        path: join(srcDir, "features", "planted.tsx"),
        area: "src",
        text: "const a = <Badge>{deal.status}</Badge>;",
      },
    ]);
    expect(rule.corpus(tree)).toEqual([]);
    expect(ruleFindings(rule, tree.files, tree)).toEqual([
      "src/features/planted.tsx: {deal.status}",
    ]);
  });
});
