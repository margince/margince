// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { expect, type Page } from "@playwright/test";
import { de } from "../src/i18n/de";
import { meFixture } from "../src/testing/mefixture";
import { expectShellRendered } from "./screens";
import { servePrincipal } from "./seed";

export const SHARED_LINK_ROWS = Array.from({ length: 3 }, (_, index) => ({
  id: `share-${index}`,
  kind: index === 1 ? "snapshot" : "live",
  target: "forecast",
  scope_kind: "workspace",
  created_at: `2026-03-0${index + 1}T09:00:00Z`,
  expires_at: `2026-04-0${index + 1}T09:00:00Z`,
}));

// The analytics forecast for a reader who may share it. The drawer opens from
// its trigger by the keyboard, so every caller starts from the same place.
// The rail offers Analytics only to a reader who may read the forecast.
export async function openSharedLinks(page: Page) {
  await servePrincipal(
    page,
    meFixture({ allow: { forecast: ["read", "create"] } }),
  );
  await page.route(/\/v1\/forecast\/shares$/, (route) =>
    route.fulfill({ json: { data: SHARED_LINK_ROWS } }),
  );
  await page.goto("/#/analytics/forecast");
  await page.waitForLoadState("networkidle");
  await expectShellRendered(page);
  const trigger = page.getByRole("button", {
    name: de["analytics.share.listOpen"],
  });
  await trigger.focus();
  await page.keyboard.press("Enter");
  await expect(
    page
      .getByRole("dialog", { name: de["analytics.share.listTitle"] })
      .getByRole("button", { name: de["analytics.share.revoke"] }),
  ).toHaveCount(SHARED_LINK_ROWS.length);
  return trigger;
}
