import { expect, test } from "@playwright/test";
import type { components } from "../src/api/schema";
import { meFixture } from "../src/testing/mefixture";
import { E2E_ADMIN_GRANTS } from "./mockapi/fixtures";
import { magicReceipt } from "./mockapi/work";
import { mockApi, servePrincipal } from "./seed";

const changes: components["schemas"]["MagicLine"][] = [
  "magic.action.create_contact",
  "magic.action.create_company",
  "magic.action.create_project",
].map((key, index) => ({
  id: `11111111-1111-7111-8111-11111111111${index}`,
  receipt_ids: [`11111111-1111-7111-8111-11111111111${index}`],
  lane: "done",
  occurred_at: "2026-11-15T08:00:00Z",
  summary: { key },
  actor: { type: "system", id: "capture" },
}));

test("three completed Home preview items remain in Change history after navigation", async ({
  page,
}) => {
  await mockApi(page);
  const principal = meFixture({ allow: E2E_ADMIN_GRANTS });
  await servePrincipal(page, {
    ...principal,
    user: { ...principal.user, locale: "en" },
  });
  await page.route(
    (url) => url.pathname === "/v1/magic",
    (route) =>
      route.fulfill({
        json: {
          ...magicReceipt,
          as_of: "2026-11-15T09:00:00Z",
          since: "2026-11-14T09:00:00Z",
          done: changes,
          totals: { done: 3, needs_you: 0, could_not_complete: 0, watching: 0 },
        },
      }),
  );
  await page.route(
    (url) => url.pathname === "/v1/worklist/handled",
    (route) =>
      route.fulfill({
        json: {
          as_of: "2026-11-15T09:00:00Z",
          since: "2026-11-14T09:00:00Z",
          truncated: false,
          receipts: changes.map((line) => ({
            id: line.id,
            kind: line.summary.key,
            summary: "",
            occurred_at: line.occurred_at,
            completed_change: line,
          })),
        },
      }),
  );
  await page.goto("/#/home");
  const preview = page.locator(".brief-done-preview");
  for (const sentence of [
    "Created contact",
    "Created company",
    "Created project",
  ]) {
    await expect(preview).toContainText(sentence);
  }
  await page.getByRole("link", { name: "Change history", exact: true }).click();
  const history = page.locator("#worklist-change-history");
  await expect(history).toBeVisible();
  await history.getByText("Show recent changes", { exact: true }).click();
  for (const sentence of [
    "Created contact",
    "Created company",
    "Created project",
  ]) {
    await expect(
      history.getByRole("cell", { name: sentence, exact: true }),
    ).toBeVisible();
  }
  await expect(
    history.getByText("No actions taken on your behalf today."),
  ).toHaveCount(0);
});
