// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { LocaleProvider } from "../../i18n";
import {
  SETTINGS_PAGES,
  visibleSettingsPages,
} from "../../lib/settingscatalog";
import { meFixture } from "../../testing/mefixture";
import { AboutPage } from "./aboutpage";

afterEach(cleanup);

describe("the About page", () => {
  it("is offered to a seat that holds no grants, last in System", () => {
    const bare = meFixture({ roles: [], allow: {} });
    const system = visibleSettingsPages(bare).filter(
      (page) => page.group === "system",
    );
    expect(system.map((page) => page.id)).toEqual(["about"]);
  });

  it("comes last in System", () => {
    const system = SETTINGS_PAGES.filter((page) => page.group === "system");
    expect(system.at(-1)?.id).toBe("about");
  });

  it("shows the version block", () => {
    render(
      <LocaleProvider initial="en">
        <AboutPage />
      </LocaleProvider>,
    );
    expect(screen.getByText("Version")).toBeInTheDocument();
  });
});
