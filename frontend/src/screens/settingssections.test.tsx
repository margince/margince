/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { composeStories } from "@storybook/react-vite";
import { render, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { translate } from "../i18n";
import type { MessageKey } from "../i18n/en";
import * as stories from "./settings.stories";
import { SETTINGS_PAGES, type SettingsPageId } from "./settingscatalog";
import { settingsPageSections } from "./settingssearch";

const t = (key: MessageKey) => translate("en", key);

// Drawn, and not offered to search: a label for an empty state, or one shown
// only to a reader holding a grant the search does not know about.
const DRAWN_NOT_OFFERED: Partial<
  Record<SettingsPageId, readonly MessageKey[]>
> = {
  connections: ["connectors.emptyLabel", "connectors.telegramEmptyLabel"],
  "capture-activity": ["captureActivity.scope.label"],
};

const pageStories = Object.values(composeStories(stories)).filter(
  (story) => story.parameters.settingsPage !== undefined,
);

// A setting name is the visible text, less its badges, of a naming primitive:
// a titled Panel's heading, a SettingRow's name, a Field's label, a legend or a
// Disclosure's summary. Not a table header: it heads a column of records.
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

function drawnNames(root: HTMLElement): Set<string> {
  const panels = [...root.querySelectorAll("section.panel[aria-labelledby]")];
  const titles = panels.map((panel) =>
    root.ownerDocument.getElementById(
      panel.getAttribute("aria-labelledby") ?? "",
    ),
  );
  const named = [...root.querySelectorAll(NAMING)];
  return new Set([...titles, ...named].map(visibleText));
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
      const view = render(<Story />);
      await waitFor(() => {
        // A page that drew no panel at all crashed or never loaded.
        expect(view.container.querySelector(".panel")).not.toBeNull();
        expect(view.container.querySelector("[aria-busy=true]")).toBeNull();
        expect(drawnNames(view.container)).toEqual(
          new Set([
            ...settingsPageSections(page, t, { holdsAdminRole: true }),
            ...(DRAWN_NOT_OFFERED[page] ?? []).map(t),
          ]),
        );
      });
    },
  );
});
