import { expect, test } from "@playwright/test";
import { loadLocale } from "../src/i18n/catalogs";
import { de } from "../src/i18n/de";
import { expectNoAaViolations, settleAnimations } from "./a11y";
import { brandt, seededProject } from "./mockapi/fixtures";
import { company360 } from "./mockapi/records";
import { mockApi } from "./seed";

test.beforeAll(async () => {
  await loadLocale("de");
});

test.beforeEach(async ({ page }) => {
  await mockApi(page);
});

// Removing a link has one look on the project page and the company page: a
// "⋯" menu in view at rest on each row, holding Remove.
test.describe("AC-project-links: each link row's menu", () => {
  const empty = { data: [], page: { next_cursor: null, has_more: false } };
  const menuFor = (name: string) =>
    de["record.moreActionsFor"].replace("{subject}", name);

  // Opens from the keyboard, holds Remove, meets AA, and Escape puts the
  // reader back on the trigger with nothing removed.
  async function walkMenu(
    page: import("@playwright/test").Page,
    name: string,
    label: string,
  ) {
    const trigger = page.getByRole("button", { name: menuFor(name) });
    await expect(trigger).toBeVisible();
    await trigger.focus();
    await page.keyboard.press("Enter");
    const remove = page.getByRole("button", {
      name: de["rel.remove"],
      exact: true,
    });
    await expect(remove).toBeVisible();
    await settleAnimations(page);
    await expectNoAaViolations(page, label);
    await page.keyboard.press("Escape");
    await expect(remove).toHaveCount(0);
    await expect(trigger).toBeFocused();
  }

  test("on a project's companies and stakeholders", async ({ page }) => {
    await page.route(/\/v1\/projects\/pr-fleet\/360(\?.*)?$/, (route) =>
      route.fulfill({
        json: {
          as_of: "2026-07-06T09:00:00Z",
          project: {
            ...seededProject,
            companies: [
              {
                company_id: brandt.id,
                display_name: brandt.display_name,
                role: "customer",
              },
              {
                company_id: "o-relay",
                display_name: "Relay Systems",
                role: "partner",
              },
            ],
          },
          sections_omitted: [],
          phase_history: { data: [], phase_durations: [] },
          deals: empty,
          stakeholders: {
            data: [
              {
                relationship_id: "rel-1",
                contact_id: "p-anna",
                contact_name: "Anna Weber",
                role: "champion",
              },
            ],
            page: { next_cursor: null, has_more: false },
          },
          contracts: empty,
          documents: empty,
          commitments: empty,
          activities: empty,
        },
      }),
    );
    await page.goto("/#/projects/pr-fleet");
    await expect(
      page.getByRole("heading", { level: 1, name: seededProject.name }),
    ).toBeVisible();
    await page.getByRole("button", { name: de["record.moreActions"] }).click();
    await page
      .getByRole("button", { name: de["project.more.contacts"] })
      .click();
    await expect(
      page.getByRole("dialog", { name: de["project.more.contacts"] }),
    ).toBeVisible();
    await walkMenu(page, "Relay Systems", "project companies — menu open");
    await walkMenu(page, "Anna Weber", "project stakeholders — menu open");
  });

  test("on a company's projects, beside the key chip", async ({ page }) => {
    await page.route(/\/v1\/companies\/o-brandt\/360(\?.*)?$/, (route) =>
      route.fulfill({
        json: {
          ...company360,
          projects: [
            {
              project_id: seededProject.id,
              name: seededProject.name,
              key: seededProject.key,
              phase: "initiative",
              quiet: false,
            },
          ],
          projects_page: { has_more: false, next_cursor: null },
        },
      }),
    );
    await page.goto("/#/companies/o-brandt/deals");
    const row = page.locator(".pl-row", { hasText: seededProject.name });
    await expect(row.locator(".chip")).toHaveText(seededProject.key ?? "");
    await walkMenu(page, seededProject.name, "company projects — menu open");
  });
});
