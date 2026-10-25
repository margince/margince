// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";
import type { Translator } from "../../i18n";
import type { Deal } from "./deals";
import { dealColumns } from "./deals.columns";

function closeColumn() {
  const t: Translator = (key: string) => key;
  const column = dealColumns(t, "en", "UTC", new Map()).find(
    (candidate) => candidate.key === "close",
  );
  if (!column) {
    throw new Error("the deals table has no close column");
  }
  return column;
}

describe("the deals table close column", () => {
  const dates = {
    closed_at: "2026-08-20T12:00:00Z",
    expected_close_date: "2026-03-02",
  };
  const deal = (status: Deal["status"]): Deal => ({
    id: "d1",
    name: "Renewal",
    pipeline_id: "p1",
    stage_id: "s1",
    source: "manual",
    captured_by: "user:u1",
    created_at: "2026-01-01T00:00:00Z",
    updated_at: "2026-01-01T00:00:00Z",
    ...dates,
    status,
  });

  it("sorts by the close day the cell shows", () => {
    expect(closeColumn().sort).toBe("close_day");
  });

  it("shows a closed deal's closed day, not its forecast", () => {
    const cell = closeColumn().cell(deal("won"));
    expect(cell).toBe("20/08/2026");
  });

  it("shows an open deal's forecast", () => {
    const cell = closeColumn().cell(deal("open"));
    expect(cell).toBe("02/03/2026");
  });
});
