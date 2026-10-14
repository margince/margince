// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  SeedInstallation,
  SHELL_ROUTES,
  ShellExample,
} from "../app/shell.storykit";
import { routeHash } from "../lib/router";
import { type GrantSpec, meFixture } from "../testing/mefixture";
import {
  installFetchStub,
  jsonResponse,
  type RouteMap,
  StoryProviders,
} from "../testing/story-utils";
import { SettingsScreen, settingsAddress } from "./settings";

/**
 * The /me body of a settings story: an admin holding exactly `allow`.
 *
 * The bare user id, as /me reports it, so an audit row by "human:u-mor" reads "You".
 */
export const me =
  (allow: GrantSpec = {}) =>
  () =>
    jsonResponse({
      ...meFixture({ roles: ["admin"], allow }),
      user: { ...meFixture().user, id: "u-mor", email: "ada@acme.test" },
    });

// Account's bookability card reads this on mount and throws without it, so it
// sits under every page. The zone is the identity fixture's, not a second one.
export const WORKING_HOURS: RouteMap = {
  "GET /me/working-hours": () =>
    jsonResponse({
      chosen: false,
      working_hours: {
        start_time: "09:00",
        end_time: "17:00",
        days: [1, 2, 3, 4, 5],
        timezone: meFixture().user.timezone,
      },
    }),
};

// The one desktop width every full-page settings story is reviewed at.
const DESKTOP = "settingsDesktop";
const DESKTOP_VIEWPORT = {
  name: "Settings page (1440px)",
  styles: { width: "1440px", height: "900px" },
};

/** The page's routes. `GET /me` is required: a page drawn without its grants
 * renders withheld and looks like a deliberate denial. */
export type SettingsPageRoutes = RouteMap & {
  "GET /me": RouteMap[string];
};

export type SettingsPageLook = Readonly<{
  theme?: "light" | "dark";
  // A catalog viewport from .storybook/preview.tsx; unset is the 1440 desktop.
  viewport?: "phone" | "phoneOnItsSide";
}>;

/**
 * A settings page as the app draws it: shell, settings nav and the page, at the
 * 1440 desktop width unless `look.viewport` names a catalog one.
 *
 * Spread it into a story (`{ ...settingsPage(...), play }`). The story's routes
 * go last so they can override the chrome's defaults.
 */
export function settingsPage(
  tabId: string,
  routes: SettingsPageRoutes,
  look: SettingsPageLook = {},
) {
  const address = settingsAddress(tabId);
  const at = routeHash(address);
  return {
    parameters: {
      layout: "fullscreen",
      viewport: { options: { [DESKTOP]: DESKTOP_VIEWPORT } },
    },
    globals: {
      theme: look.theme ?? "light",
      viewport: { value: look.viewport ?? DESKTOP },
    },
    render: () => {
      installFetchStub({ ...SHELL_ROUTES, ...WORKING_HOURS, ...routes });
      return (
        <StoryProviders>
          <SeedInstallation>
            <ShellExample at={at}>
              <SettingsScreen route={address} />
            </ShellExample>
          </SeedInstallation>
        </StoryProviders>
      );
    },
  };
}
