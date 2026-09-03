// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { describe, expect, it } from "vitest";

import { en } from "../i18n/en";
import { NAV } from "./nav";
import { OFF_RAIL_TITLE_KEYS, SELF_HEADED_SCREENS } from "./pagemeta";
import { SCREENS } from "./router";

// `resolveTitle` falls through to `shell.unknownPage` for a screen that is
// neither on the NAV rail nor in OFF_RAIL_TITLE_KEYS, so a routed destination
// missing from both is headed "Not found" — above whatever it just found. The
// tag page shipped that way.
//
// The corpus is DERIVED from the router's own screen list, so a destination
// added there is covered here without anybody remembering to add it.

// Screens the shell never heads. The self-headed set is the PRODUCT'S own —
// read from `pagemeta` rather than copied, so a screen that starts heading
// itself leaves this gate without also having to be listed here. What is
// spelled below is only the surfaces that render OUTSIDE the shell entirely:
// pre-auth and public pages, which have no page heading to resolve, plus
// `not-found`, which IS the unknown page rather than one missing its name.
const OUTSIDE_THE_SHELL = new Set<string>([
  "onboarding",
  "client",
  "book",
  "preferences",
  "unsubscribe",
  "confirm",
  "room",
  "oauth-consent",
  "reset-password",
  "not-found",
  // The fork widening point: `#/x/<key>` destinations name themselves from
  // the unit's own descriptor, which the shell resolves at runtime.
  "x",
  "ext",
]);

const HEADS_ITSELF = (screen: string) =>
  OUTSIDE_THE_SHELL.has(screen) || SELF_HEADED_SCREENS.has(screen);

describe("every destination the shell heads has a name", () => {
  const railScreens = new Set(NAV.map((item) => item.screen));

  it.each(SCREENS.filter((screen) => !HEADS_ITSELF(screen)))(
    "%s is named by the rail or by the off-rail map",
    (screen) => {
      const named = railScreens.has(screen) || screen in OFF_RAIL_TITLE_KEYS;
      expect(named).toBe(true);
    },
  );

  // The off-rail map's keys are real copy, not slugs typed twice.
  it.each(Object.entries(OFF_RAIL_TITLE_KEYS))(
    "%s resolves to a translated name",
    (_screen, key) => {
      expect(en[key]).toBeTruthy();
    },
  );
});
