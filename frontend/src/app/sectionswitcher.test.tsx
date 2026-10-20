/** @vitest-environment happy-dom */
// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import "@testing-library/jest-dom/vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it } from "vitest";
import {
  SETTINGS_SCREEN,
  useSettingsSection,
} from "../features/settings/settingsnav";
import { LocaleProvider, translate } from "../i18n";
import type { Route } from "../lib/router";
import { meFixture } from "../testing/mefixture";
import { sectionHead } from "./pagemeta";
import { SectionSwitcher } from "./sectionswitcher";

afterEach(() => {
  cleanup();
  window.location.hash = "";
});

const ROUTE: Route = { screen: SETTINGS_SCREEN, id: "tags" };

function Switcher() {
  const head = sectionHead(useSettingsSection(ROUTE), ROUTE);
  return head ? (
    <SectionSwitcher section={head.section} entry={head.entry} />
  ) : null;
}

function mount() {
  const client = new QueryClient();
  client.setQueryData(
    ["me"],
    meFixture({ roles: ["manager"], allow: { tag: ["read", "update"] } }),
  );
  render(
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">
        <Switcher />
      </LocaleProvider>
    </QueryClientProvider>,
  );
}

const trigger = () =>
  screen.getByRole("button", {
    name: translate("en", "shell.sectionSwitch", {
      name: translate("en", "settings.tab.tags"),
    }),
  });
const searchBox = () => screen.getByRole("combobox");

async function openSheet(user: ReturnType<typeof userEvent.setup>) {
  await user.click(trigger());
  await screen.findByRole("dialog");
}

async function expectClosedOnTrigger() {
  await waitFor(() => expect(screen.queryByRole("dialog")).toBeNull());
  await waitFor(() => expect(trigger()).toHaveFocus());
}

describe("the phone section switcher", () => {
  it("closes on Escape from the current row and hands focus back", async () => {
    const user = userEvent.setup();
    mount();
    await openSheet(user);
    expect(
      screen.getByRole("link", {
        name: translate("en", "settings.tab.tags"),
        current: "page",
      }),
    ).toHaveFocus();

    await user.keyboard("{Escape}");

    await expectClosedOnTrigger();
  });

  it("closes on Escape from an empty search box", async () => {
    const user = userEvent.setup();
    mount();
    await openSheet(user);
    searchBox().focus();

    await user.keyboard("{Escape}");

    await expectClosedOnTrigger();
  });

  it("spends the first Escape on typed text and the second on the sheet", async () => {
    const user = userEvent.setup();
    mount();
    await openSheet(user);
    await user.type(searchBox(), "tag");

    await user.keyboard("{Escape}");

    expect(searchBox()).toHaveValue("");
    expect(searchBox()).toHaveFocus();
    expect(screen.getByRole("dialog")).toBeInTheDocument();

    await user.keyboard("{Escape}");

    await expectClosedOnTrigger();
  });
});
