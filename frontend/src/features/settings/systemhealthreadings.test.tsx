// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { LocaleProvider } from "../../i18n";
import { attentionReport } from "./systemhealth.fixtures";
import { HealthReadings } from "./systemhealthreadings";

afterEach(cleanup);

it("draws labeled state colors from authorized checks without counting hidden services", () => {
  render(
    <LocaleProvider initial="en">
      <HealthReadings
        sources={[
          ...attentionReport.sources,
          {
            id: "hidden",
            availability: "withheld",
            condition: "unknown",
            code: "withheld",
            scope: "workspace",
            permission: "job_health:read",
          },
        ]}
        findings={attentionReport.findings}
      />
    </LocaleProvider>,
  );
  const failed = screen.getByRole("button", { name: "Failed checks: 1" });
  expect(failed).toHaveAttribute("data-tone", "danger");
  expect(
    screen.getByRole("button", { name: "Passing checks: 5" }),
  ).toHaveAttribute("data-tone", "success");
  expect(
    screen.getByRole("button", { name: "Need a decision: 0" }),
  ).toHaveAttribute("data-tone", "warning");
});
