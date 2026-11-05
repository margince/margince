/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, render, screen } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, it } from "vitest";
import { UTC_ZONE } from "../../format/timezone";
import { LocaleProvider, translate } from "../../i18n";
import type { Deal } from "./deals";
import { dealColumns } from "./deals.columns";

afterEach(cleanup);

const deal = (status: Deal["status"]): Deal => ({
  id: "d1",
  name: "Renewal",
  pipeline_id: "p1",
  stage_id: "s1",
  source: "manual",
  captured_by: "user:u1",
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
  status,
});

function statusCell(row: Deal): ReactNode {
  const column = dealColumns(
    (key) => translate("vi", key),
    "vi",
    UTC_ZONE,
    new Map(),
  ).find((candidate) => candidate.key === "status");
  if (!column) {
    throw new Error("the deals table has no status column");
  }
  return column.cell(row);
}

it("names a deal's status in the reader's language, never the stored value", () => {
  render(
    <LocaleProvider initial="vi">
      {statusCell(deal("open"))}
      {statusCell(deal("lost"))}
    </LocaleProvider>,
  );
  expect(screen.getByText("Đang mở")).toBeTruthy();
  expect(screen.getByText("Thua")).toBeTruthy();
  expect(screen.queryByText("open")).toBeNull();
});
