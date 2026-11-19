// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

/** @vitest-environment happy-dom */
import "@testing-library/jest-dom/vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { LocaleProvider } from "../../i18n";
import {
  SETTINGS_PAGES,
  visibleSettingsPages,
} from "../../lib/settingscatalog";
import { meFixture } from "../../testing/mefixture";
import { AboutPage } from "./aboutpage";

vi.mock("../../lib/release", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../lib/release")>()),
  SPA_RELEASE: "2026.2.1",
  CORE_RELEASE: "",
  BUILD_REVISION: "0123456789abcdef0123456789abcdef01234567",
}));

afterEach(cleanup);

describe("the About page", () => {
  it("is the one System page a seat with no grants opens, and the last one in System", () => {
    const bare = meFixture({ roles: [], allow: {} });
    const system = visibleSettingsPages(bare).filter(
      (page) => page.group === "system",
    );
    expect(system.map((page) => page.id)).toEqual(["about"]);
    const all = SETTINGS_PAGES.filter((page) => page.group === "system");
    expect(all.at(-1)?.id).toBe("about");
  });

  it("shows the release and commit this bundle was stamped with", () => {
    render(
      <LocaleProvider initial="en">
        <AboutPage />
      </LocaleProvider>,
    );
    expect(
      screen.getByRole("button", { name: /^2026\.2\.1 · 0123456/ }),
    ).toBeVisible();
  });
});
