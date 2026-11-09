// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { rule } from "./recordtaborder.census";

const planted = (text: string) =>
  plantedFindings(rule, [
    { path: join(srcDir, "features", "x", "page.tsx"), area: "src", text },
  ]);

describe("the recordtaborder rule", () => {
  it("passes a typed list in the shared order", () => {
    expect(
      planted(
        'const TABS = ["overview", "timeline", "deals"] satisfies RecordTabId[];',
      ),
    ).toEqual([]);
  });

  it("fails an untyped strip, a strip out of order, and one not on Overview", () => {
    expect(planted('const TABS = ["overview", "deals", "timeline"];')).toEqual([
      "src/features/x/page.tsx TABS is not typed as a record tab list",
      "src/features/x/page.tsx TABS does not follow RECORD_TAB_ORDER",
    ]);
    expect(
      planted('const TABS = ["deals", "overview"] satisfies RecordTabId[];'),
    ).toEqual([
      "src/features/x/page.tsx TABS does not open on Overview",
      "src/features/x/page.tsx TABS does not follow RECORD_TAB_ORDER",
    ]);
  });

  it("leaves an array that is no tab strip alone", () => {
    expect(planted('const KINDS = ["overview", "unrelated"];')).toEqual([]);
  });
});
