/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { composeStories } from "@storybook/react-vite";
import { render, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { translate } from "../i18n";
import type { MessageKey } from "../i18n/en";
import * as stories from "./settings.stories";
import { SETTINGS_PAGES } from "./settingscatalog";
import { settingsPageSections } from "./settingssearch";

const t = (key: MessageKey) => translate("en", key);

const pageStories = Object.values(composeStories(stories)).filter(
  (story) => story.parameters.settingsPage !== undefined,
);

// A titled Panel is a region named by its title; an untitled one names nothing.
function drawnNames(root: HTMLElement): Set<string> {
  const panels = [...root.querySelectorAll("section.panel[aria-labelledby]")];
  const titles = panels.map((panel) => {
    const titleId = panel.getAttribute("aria-labelledby") ?? "";
    return root.ownerDocument.getElementById(titleId)?.textContent ?? "";
  });
  const rows = [...root.querySelectorAll(".settingrow-naming > .t-name")];
  return new Set([...titles, ...rows.map((row) => row.textContent ?? "")]);
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
          new Set(settingsPageSections(page, t)),
        );
      });
    },
  );
});
