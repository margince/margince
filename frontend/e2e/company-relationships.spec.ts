// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, test } from "@playwright/test";
import { expectNoAaViolations } from "./a11y";
import { brandt } from "./mockapi/fixtures";
import { page as listPage } from "./mockapi/server";
import { mockApi } from "./seed";
import { expectShellRendered, pageOverflow, settleAnimations } from "./waits";

test.use({ viewport: { width: 390, height: 844 } });

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

test("AC-company-relationships: a company's relationship table folds at 390 and keeps each date whole", async ({
  page,
}) => {
  await page.route(/\/v1\/relationships(\?.*)?$/, (route) =>
    route.fulfill({
      json: listPage([
        {
          id: "r-anna",
          kind: "employment",
          company_id: brandt.id,
          contact_id: "p-anna",
          role: "Leiterin Einkauf und strategische Beschaffung",
          started_at: "2021-03-01",
          ended_at: "2024-11-30",
          is_current_primary: false,
          employment_status: "former",
          version: 1,
          created_at: "2026-01-01T00:00:00Z",
          updated_at: "2026-06-01T00:00:00Z",
        },
      ]),
    }),
  );
  await page.goto(`/#/companies/${brandt.id}/profile`);
  await page.waitForLoadState("networkidle");
  await expectShellRendered(page);
  const dates = page.locator(".relationshiprows-date");
  await expect(dates).toHaveCount(2);
  await settleAnimations(page);
  expect(await pageOverflow(page)).toEqual([]);
  const pastScrollBox = await page.evaluate(() =>
    Array.from(document.querySelectorAll(".table-scroll")).flatMap((box) =>
      Array.from(box.querySelectorAll("td, th"))
        .filter(
          (cell) =>
            cell.getBoundingClientRect().right >
            box.getBoundingClientRect().right + 1,
        )
        .map((cell) => (cell.textContent ?? "").trim()),
    ),
  );
  expect(pastScrollBox).toEqual([]);
  const lineBoxes = await dates.evaluateAll((nodes) =>
    nodes.map(
      (node) =>
        new Set(
          Array.from(node.getClientRects(), (rect) => Math.round(rect.top)),
        ).size,
    ),
  );
  expect(lineBoxes).toEqual([1, 1]);
  await expectNoAaViolations(page, "company profile, relationships (390px)");
});
