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
import {
  SETTINGS_PAGES,
  type SettingsPageId,
  visibleSettingsTabs,
} from "../../lib/settingscatalog";
import { settingsHref } from "../../lib/settingsrouting";
import { meFixture } from "../../testing/mefixture";
import * as stories from "./settings.stories";
import { catalogContext } from "./settingsnav";
import { COMPANY_CONTEXT_OFF, EVERY_GRANT } from "./settingspages.fixtures";
import {
  COMPANY_CONTEXT_SECTIONS,
  settingsSectionKeys,
} from "./settingssections";
import { settingsTabLabel } from "./settingstabs";
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

// Keyed by page, or by `page/tab` for a name one tab draws.
const DRAWN_NOT_OFFERED: Readonly<Record<string, readonly NotOffered[]>> = {
  members: [{ key: "users.search", why: "record-filter" }],
  "connections/activity": [
    { key: "captureActivity.scope.label", why: "grant" },
  ],
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
  await user.click(
    await screen.findByRole("button", { name: t("health.diagnostics") }),
  );
  // The tab strip routes, so each tab below is a fresh render of the page.
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
  await user.click(
    screen.getByRole("button", { name: t("health.configuration") }),
  );
  await waitFor(() => {
    expect(
      drawnNames(view.container).has(t("operations.schedules.title")),
    ).toBe(true);
    expect(view.container.querySelector("[aria-busy=true]")).toBeNull();
  });
  for (const name of drawnNames(view.container)) names.add(name);
  return names;
}

// Every tab the stories' admin may open, or the page itself when it has none.
// A tab offering composed units opens only in a build that composed some. This
// build's context decides it as the strip does.
function tabsOf(page: SettingsPageId): readonly (string | undefined)[] {
  const row = SETTINGS_PAGES.find((each) => each.id === page);
  if (row === undefined || !("tabs" in row)) return [undefined];
  const admin = meFixture({ roles: ["admin"], allow: EVERY_GRANT });
  const context = { ...catalogContext(), companyContextShown: true };
  return visibleSettingsTabs(row, admin, context).map((tab) => tab.id);
}

// The names search offers for one tab, less the tab's own name: the strip
// draws that, not a panel.
function drawnSections(page: SettingsPageId, tab: string | undefined) {
  const keys = settingsSectionKeys(
    page,
    (each) => tab === undefined || each === tab,
  );
  return [
    ...keys.filter((key) => !TAB_NAMES.has(key)),
    ...(page === "company" && (tab === undefined || tab === "profile")
      ? COMPANY_CONTEXT_SECTIONS
      : []),
  ].map(t);
}

const TAB_NAMES = new Set<string>(
  SETTINGS_PAGES.flatMap((page) =>
    "tabs" in page
      ? page.tabs.flatMap((tab) => settingsTabLabel(page.id, tab.id) ?? [])
      : [],
  ),
);

async function expectTabDraws(
  Story: (typeof pageStories)[number],
  page: SettingsPageId,
  tab: string | undefined,
) {
  const address = tab === undefined ? page : `${page}/${tab}`;
  const notOffered = [
    ...(DRAWN_NOT_OFFERED[page] ?? []),
    ...(tab === undefined ? [] : (DRAWN_NOT_OFFERED[address] ?? [])),
  ];
  const view = render(<Story route={settingsHref(page, tab)} />);
  await waitFor(() => {
    // A tab that drew no panel at all crashed or never loaded.
    expect(view.container.querySelector(".panel"), address).not.toBeNull();
    expect(view.container.querySelector("[aria-busy=true]")).toBeNull();
    // Sorted lists rather than sets, so a failure names the odd one out.
    expect([...drawnNames(view.container)].sort(), address).toEqual(
      [
        ...new Set([
          ...drawnSections(page, tab),
          ...notOffered.map(({ key }) => t(key)),
        ]),
      ].sort(),
    );
  });
  const filters = notOffered
    .filter(({ why }) => why === "record-filter")
    .map(({ key }) => t(key));
  for (const element of namingElements(view.container)) {
    if (filters.includes(visibleText(element))) {
      expect(element.closest(".filter-bar")).not.toBeNull();
    }
  }
  view.unmount();
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
          new Set(drawnSections(page, undefined)),
        );
        return;
      }
      for (const tab of tabsOf(page)) {
        await expectTabDraws(Story, page, tab);
      }
    },
  );

  it("opens Company profile on its next tab while the rollout hides the card", async () => {
    const seesCompanyContext = companyContextShown(true, COMPANY_CONTEXT_OFF);
    expect(seesCompanyContext).toBe(false);
    const { CompanyPageContextOff } = composeStories(stories);
    render(<CompanyPageContextOff />);
    // The profile tab draws only that card, so it leaves the strip and the
    // page's bare address opens the legal entities instead.
    expect(
      await screen.findByRole("button", {
        name: t("settings.part.company.entities"),
        pressed: true,
      }),
    ).toBeTruthy();
    expect(
      screen.queryByRole("button", {
        name: t("settings.part.company.profile"),
      }),
    ).toBeNull();
    for (const key of COMPANY_CONTEXT_SECTIONS) {
      expect(screen.queryByText(t(key))).toBeNull();
    }
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
