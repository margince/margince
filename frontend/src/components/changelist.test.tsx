/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { cleanup, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { render } from "../testing/brief.testkit";
import { ChangeList } from "./changelist";

afterEach(cleanup);

const stageId = "0c000000-0000-4000-8000-000000000001";

it("reads money in its currency, a stage by its name and a day as a date", () => {
  render(
    <ChangeList
      changes={[
        {
          field: "amount_minor",
          type: "money",
          currency: "EUR",
          before: 1_250_000,
          after: 3_000_000,
        },
        {
          field: "stage_id",
          type: "stage",
          before: stageId,
          after: "0c000000-0000-4000-8000-000000000002",
          before_label: "Qualified",
          after_label: "Closed lost",
        },
        {
          field: "expected_close_date",
          type: "date",
          precision: "day",
          before: "2026-09-30",
          after: "2026-10-31",
        },
      ]}
    />,
  );
  const text = document.body.textContent ?? "";
  expect(text).toContain("€12,500.00");
  expect(text).toContain("€30,000.00");
  expect(text).toContain("Qualified → Closed lost");
  expect(text).toContain("30/09/2026");
  expect(text).not.toContain(stageId);
  expect(text).not.toContain("1250000");
  expect(screen.getByText("Stage")).toBeTruthy();
});

it("never prints an id, minor units or a code it cannot name", () => {
  render(
    <ChangeList
      changes={[
        { field: "owner_id", type: "member", before: stageId, after: null },
        { field: "lost_reason", type: "enum", before: null, after: "silent" },
        { field: "amount_minor", before: 100, after: 200 },
        { field: "company_id", before: stageId, after: stageId },
      ]}
    />,
  );
  const text = document.body.textContent ?? "";
  expect(text).not.toContain(stageId);
  expect(text).not.toContain("silent");
  expect(text).not.toMatch(/\b(100|200)\b/);
  expect(text).toContain("hidden");
});
