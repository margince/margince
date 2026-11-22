// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { plantedFindings, srcDir } from "../tree";
import { rule } from "./company-page-style.census";

const companies = join(srcDir, "features", "companies");
const records = join(srcDir, "features", "records");

describe("the company-page-style rule", () => {
  it("fails a second date or money spelling on the company page", () => {
    const text =
      'import { formatDateTime, formatMoney } from "../../format/format";\n' +
      "const sum = [1].map(formatMoney);\n";
    const findings = plantedFindings(rule, [
      { path: join(companies, "panel.tsx"), area: "src", text },
      { path: join(records, "historyentries.tsx"), area: "src", text },
    ]);
    expect(findings).toEqual([
      "src/features/companies/panel.tsx: formatDateTime (use formatDateTimeAbbrev)",
      "src/features/companies/panel.tsx: formatMoney (use formatMoneyWhole)",
      "src/features/records/historyentries.tsx: formatDateTime (use formatDateTimeAbbrev)",
      "src/features/records/historyentries.tsx: formatMoney (use formatMoneyWhole)",
    ]);
  });

  it("spares the page's own spelling, comments, tests and other records files", () => {
    const text = 'import { formatDate } from "../../format/format";\n';
    const findings = plantedFindings(rule, [
      {
        path: join(companies, "panel.tsx"),
        area: "src",
        text: "// formatDate is the numeric form.\nformatDateAbbrev(at, locale, zone);\n",
      },
      { path: join(companies, "panel.test.tsx"), area: "src", text },
      { path: join(records, "customfields.tsx"), area: "src", text },
    ]);
    expect(findings).toEqual([]);
  });
});
