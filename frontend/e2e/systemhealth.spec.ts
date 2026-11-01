// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, test } from "@playwright/test";
import {
  attentionReport,
  HEALTH_AT,
} from "../src/features/settings/systemhealth.fixtures";
import { meFixture } from "../src/testing/mefixture";
import { mockApi } from "./seed";

test.use({
  locale: "en-US",
  permissions: ["clipboard-read", "clipboard-write"],
});
test.beforeEach(async ({ page }) => {
  await mockApi(page);
  await page.route("**/v1/me", (route) =>
    route.fulfill({
      json: meFixture({
        allow: {
          job_health: ["read"],
          installation_settings: ["read", "update"],
          embedding_reindex: ["read"],
          data_task: ["read"],
          ai_diagnostics: ["read"],
        },
      }),
    }),
  );
  await page.route("**/v1/admin/system-health", (route) =>
    route.fulfill({ json: attentionReport }),
  );
  await page.route("**/v1/admin/job-health", (route) =>
    route.fulfill({
      json: {
        generated_at: HEALTH_AT,
        dead_window_hours: 24,
        kinds: [],
        recent_failures: [],
      },
    }),
  );
  await page.route("**/v1/admin/job-health/history*", (route) =>
    route.fulfill({
      json: {
        observed_at: HEALTH_AT,
        cutoff: HEALTH_AT,
        window_start: HEALTH_AT,
        groups: [
          {
            kind: "capture_classify",
            state: "discarded",
            fleet_wide: false,
            count: 1,
            first_at: HEALTH_AT,
            latest_at: HEALTH_AT,
          },
        ],
        runs: [
          {
            job_id: 4711,
            kind: "capture_classify",
            state: "discarded",
            attempt: 3,
            max_attempts: 3,
            failed_at: HEALTH_AT,
            reason: "The job could not finish.",
          },
        ],
        has_more: false,
        population_may_have_changed: false,
        coverage: "retained_only",
      },
    }),
  );
});

test("Overview loads only summary evidence, then opens retained history and a safe handoff", async ({
  page,
}) => {
  const reads: string[] = [];
  page.on("request", (r) => {
    if (r.method() === "GET") reads.push(new URL(r.url()).pathname);
  });
  const start = performance.now();
  await page.goto("/#/settings/system-health");
  await expect(
    page.getByRole("heading", {
      name: "Background work is delayed",
      exact: true,
    }),
  ).toBeVisible();
  console.log(
    `Production cold Overview: ${performance.now() - start} ms; Chromium desktop 1280x800; unthrottled local network; synthetic API boundary`,
  );
  expect(reads).toContain("/v1/admin/system-health");
  expect(reads).not.toContain("/v1/admin/job-health");
  expect(reads).not.toContain("/v1/admin/capture-health");
  await page
    .getByRole("button", { name: "See problem and next step", exact: true })
    .first()
    .click();
  await expect(
    page.getByRole("heading", {
      name: "Retained failure history",
      exact: true,
    }),
  ).toBeVisible();
  await expect(
    page.getByText("The job could not finish.", { exact: true }).first(),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Copy technical report", exact: true })
    .first()
    .click();
  await expect(
    page.getByRole("button", { name: "Copied", exact: true }),
  ).toBeVisible();
  const copied = JSON.parse(
    await page.evaluate(() => navigator.clipboard.readText()),
  );
  expect(copied.source).toBe("jobs");
  expect(copied.history.runs[0]).toMatchObject({
    job_id: 4711,
    attempt: 3,
    max_attempts: 3,
    reason: "The job could not finish.",
  });
  await page.goBack();
  await expect(
    page.getByRole("heading", { name: "Attention needed", exact: true }),
  ).toBeVisible();
  await page.goForward();
  await expect(
    page.getByRole("heading", {
      name: "Retained failure history",
      exact: true,
    }),
  ).toBeVisible();
});

test("a settings search address focuses its control and changing views protects the draft", async ({
  page,
}) => {
  await page.goto(
    "/#/settings/system-health/configuration?section=operations.retention.label",
  );
  const input = page.getByTestId("operation-retention_sweep_interval_seconds");
  await expect(input).toBeFocused();
  await input.fill("7200");
  await input.press("Tab");
  await page.getByRole("button", { name: "Overview", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText("Save your changes?");
  await dialog.getByRole("button", { name: "Stay", exact: true }).click();
  await expect(input).toHaveValue("7200");
  await page.getByRole("button", { name: "Overview", exact: true }).click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Discard", exact: true })
    .click();
  await expect(
    page.getByRole("heading", { name: "Attention needed", exact: true }),
  ).toBeVisible();
});
