// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import { type Translator, translate } from "../../i18n";
import { employmentDateFaults } from "./employmentstatusfields";

const t: Translator = (key, params) => translate("en", key, params);
const today = "2026-11-25";

function endFault(end: string, status: "current" | "former" | "") {
  return employmentDateFaults("en", t, { start: "", end, status, today }).get(
    "end",
  );
}

describe("the end date of a job", () => {
  it("refuses a day after today for a former job", () => {
    expect(endFault("2099-12", "former")).toBe(
      "End date cannot be in the future.",
    );
    expect(endFault("2026-11-26", "former")).toBe(
      "End date cannot be in the future.",
    );
  });

  it("takes today, this month and any earlier day for a former job", () => {
    expect(endFault("2026-11-25", "former")).toBeUndefined();
    expect(endFault("2026-11", "former")).toBeUndefined();
    expect(endFault("2015-06", "former")).toBeUndefined();
  });

  it("takes a day ahead for a current job, which is a notice period", () => {
    expect(endFault("2099-12", "current")).toBeUndefined();
    expect(endFault("2099-12", "")).toBeUndefined();
  });
});
