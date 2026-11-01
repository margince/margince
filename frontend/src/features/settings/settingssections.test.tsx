/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { composeStories } from "@storybook/react-vite";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { translate } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { companyContextShown } from "../../lib/companycontextreach";
import { navigate } from "../../lib/router";
import { SETTINGS_PAGES, type SettingsPageId } from "../../lib/settingscatalog";
import { settingsHref } from "../../lib/settingsrouting";
import * as stories from "./settings.stories";
import { COMPANY_CONTEXT_OFF } from "./settingspages.fixtures";
import { settingsPageSections } from "./settingssearch";
import { healthSourceLabels } from "./systemhealth.copy";
import { healthReport } from "./systemhealth.fixtures";
import { healthSectionForSource } from "./systemhealthnavigation";

const t = (key: MessageKey) => translate("en", key);

// Drawn, and not offered to search. Each is an empty state's label, one decided
// by a grant the search cannot see, or a field that filters a record list.
type NotOffered = Readonly<{
  key: MessageKey;
  why: "empty-state" | "grant" | "record-filter";
}>;

const DRAWN_NOT_OFFERED: Partial<
  Record<SettingsPageId, readonly NotOffered[]>
> = {
  members: [{ key: "users.search", why: "record-filter" }],
  "capture-activity": [{ key: "captureActivity.scope.label", why: "grant" }],
  "model-calls": [
    { key: "aicalls.filter.search", why: "record-filter" },
    { key: "aicalls.filter.window", why: "record-filter" },
    { key: "aicalls.col.outcome", why: "record-filter" },
    { key: "aicalls.col.model", why: "record-filter" },
    { key: "aicalls.col.feature", why: "record-filter" },
  ],
  audit: [
    { key: "settings.auditActor", why: "record-filter" },
    { key: "settings.auditEntity", why: "record-filter" },
    { key: "settings.auditAction", why: "record-filter" },
    { key: "settings.auditEntityId", why: "record-filter" },
    { key: "settings.auditFrom", why: "record-filter" },
    { key: "settings.auditTo", why: "record-filter" },
  ],
};

const pageStories = Object.values(composeStories(stories)).filter(
  (story) => story.parameters.settingsPage !== undefined,
);

// Not a table header: it heads a column of records.
const NAMING = [
  ".settingrow-naming > .t-name",
  ".field > label:not(.sr-only)",
  ".field-label-row > label:not(.sr-only)",
  "legend:not(.sr-only)",
  ".disclosure-summary > .disclosure-label",
].join(", ");

function visibleText(element: Element | null): string {
  const copy = element?.cloneNode(true);
  if (!(copy instanceof Element)) {
    return "";
  }
  for (const hidden of copy.querySelectorAll("[aria-hidden], .badge")) {
    hidden.remove();
  }
  return copy.textContent?.trim() ?? "";
}

function namingElements(root: HTMLElement): Element[] {
  const panels = [...root.querySelectorAll("section.panel[aria-labelledby]")];
  const titles = panels.flatMap(
    (panel) =>
      root.ownerDocument.getElementById(
        panel.getAttribute("aria-labelledby") ?? "",
      ) ?? [],
  );
  return [...titles, ...root.querySelectorAll(NAMING)];
}

const drawnNames = (root: HTMLElement) =>
  new Set(namingElements(root).map(visibleText));

async function healthNames(Story: (typeof pageStories)[number]) {
  navigate(settingsHref("system-health"));
  const user = userEvent.setup();
  const view = render(<Story />);
  const names = new Set<string>();
  await user.click(await screen.findByRole("button", { name: "Diagnostics" }));
  for (const source of healthReport.sources) {
    const key = healthSectionForSource(source.id);
    if (!key) throw new Error(`Missing health section for ${source.id}`);
    await user.click(
      await screen.findByRole("button", {
        name: t(healthSourceLabels[source.id]),
      }),
    );
    await waitFor(() => {
      expect(drawnNames(view.container).has(t(key))).toBe(true);
      expect(view.container.querySelector("[aria-busy=true]")).toBeNull();
    });
    for (const name of drawnNames(view.container)) names.add(name);
  }
  await user.click(screen.getByRole("button", { name: "Configuration" }));
  await waitFor(() => {
    expect(
      drawnNames(view.container).has(t("operations.schedules.title")),
    ).toBe(true);
    expect(view.container.querySelector("[aria-busy=true]")).toBeNull();
  });
  for (const name of drawnNames(view.container)) names.add(name);
  return names;
}

describe("each settings page's sections are the names it draws", () => {
  it("has one page story for every catalog page", () => {
    const covered = pageStories.map((story) => story.parameters.settingsPage);
    expect(covered.sort()).toEqual(
      SETTINGS_PAGES.map((page) => page.id).sort(),
    );
  });

  it.each(SETTINGS_PAGES.map((page) => page.id))(
    "%s lists every panel and setting it draws, and nothing else",
    async (page) => {
      const Story = pageStories.find(
        (story) => story.parameters.settingsPage === page,
      );
      if (Story === undefined) {
        throw new Error(`no story in settings.stories.tsx renders ${page}`);
      }
      if (page === "system-health") {
        expect(await healthNames(Story)).toEqual(
          new Set(settingsPageSections(page, t, { seesCompanyContext: true })),
        );
        return;
      }
      const view = render(<Story />);
      await waitFor(() => {
        // A page that drew no panel at all crashed or never loaded.
        expect(view.container.querySelector(".panel")).not.toBeNull();
        expect(view.container.querySelector("[aria-busy=true]")).toBeNull();
        expect(drawnNames(view.container)).toEqual(
          new Set([
            ...settingsPageSections(page, t, { seesCompanyContext: true }),
            ...(DRAWN_NOT_OFFERED[page] ?? []).map(({ key }) => t(key)),
          ]),
        );
      });
      const filters = (DRAWN_NOT_OFFERED[page] ?? [])
        .filter(({ why }) => why === "record-filter")
        .map(({ key }) => t(key));
      for (const element of namingElements(view.container)) {
        if (filters.includes(visibleText(element))) {
          expect(element.closest(".filter-bar")).not.toBeNull();
        }
      }
    },
  );

  it("offers none of the company-context names while the rollout hides the card", async () => {
    const seesCompanyContext = companyContextShown(true, COMPANY_CONTEXT_OFF);
    expect(seesCompanyContext).toBe(false);
    const { CompanyPageContextOff } = composeStories(stories);
    const view = render(<CompanyPageContextOff />);
    await waitFor(() => {
      expect(view.container.querySelector("[aria-busy=true]")).toBeNull();
      expect(drawnNames(view.container)).toEqual(
        new Set(settingsPageSections("company", t, { seesCompanyContext })),
      );
    });
  });

  // An empty state or a grant cannot be checked from one render. Each entry
  // hides a name from search, so a new one takes an edit here.
  it("pins the unchecked exceptions to one", () => {
    const unchecked = Object.values(DRAWN_NOT_OFFERED)
      .flatMap((entries) => entries ?? [])
      .filter(({ why }) => why !== "record-filter");
    expect(unchecked).toHaveLength(1);
  });
});
