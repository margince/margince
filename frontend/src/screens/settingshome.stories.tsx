// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { meFixture } from "../app/mefixture";
import { settingsReach } from "./settingscatalog";
import { SettingsBoundary, SettingsHome } from "./settingshome";
import { installFetchStub, jsonResponse, StoryProviders } from "./story-utils";

// The settings address with NO page segment, and the two things it answers when
// the segment it does carry names no page this reader can open.
//
// Neither surface is a catalog page — `SETTINGS_HOME_ID` is deliberately not a
// member of `SETTINGS_PAGES` — so both live under one node here rather than
// under a `<Group>/<Page>` that would be a claim about a page that does not
// exist. `settingsstories.test.ts` names this title for that reason.
//
// The partition the home draws is REAL: `settingsReach` is the same evaluator
// the screen and the sidebar read, run over the same snapshot the story serves
// on `GET /me`. A hand-listed pair of page arrays would let the catalog move
// while these frames went on describing the product it used to be — and it
// would let the four panels disagree with the access block under them, which is
// the one thing this screen exists to keep in step.
//
// Read every frame in BOTH themes with the toolbar's Theme control. The panels
// are ruled lists whose hairlines and grounds are `color-mix()` of canonical
// tokens, so a surface can be correct in light and wrong in dark.

/** No composed unit in the catalog: a story installs no extension tier. */
const NO_UNITS = {};

/**
 * One principal, serving both halves of the screen.
 *
 * The reach and the `/me` body come from a single fixture on purpose: the home's
 * panels are what this seat may act on and only look up, and the block at its
 * foot is who this seat IS. Built from two snapshots they could disagree, which
 * is exactly the defect the screen's own partition exists to prevent.
 */
function home(spec: Parameters<typeof meFixture>[0]) {
  return () => {
    const snapshot = meFixture(spec);
    installFetchStub({ "GET /me": () => jsonResponse(snapshot) });
    return (
      <StoryProviders>
        <div className="wrap">
          <SettingsHome reach={settingsReach(snapshot, NO_UNITS)} />
        </div>
      </StoryProviders>
    );
  };
}

/** The boundary renders at a settings address and reads no session of its own. */
function boundary(kind: "denied" | "unknown") {
  return () => {
    installFetchStub({});
    return (
      <StoryProviders>
        <div className="wrap">
          <SettingsBoundary kind={kind} />
        </div>
      </StoryProviders>
    );
  };
}

// A team lead holding some of the company's settings and reading two more. All
// four panels stand: the personal pages, what this seat can change, what it can
// only consult, and who it is here. The grants are spelled rather than "every
// object", because a seat that can act on everything empties the "look up"
// panel — and that panel is the one carrying the screen's whole argument, that a
// page dropped from the rail is not a page taken away.
const LEAD = {
  roles: ["manager"],
  seat: "full",
  rowScope: "team",
  allow: {
    voice_profile: ["read", "create", "update"],
    pipeline: ["read", "create", "update"],
    tag: ["read", "create", "update"],
    license: ["read"],
    installation_settings: ["read"],
    // Read only, so this page lands in the half a reader may consult and not
    // change. Without one such grant the third panel is legitimately absent and
    // the frame would be the floor below wearing a different name.
    custom_field: ["read"],
  },
} as const satisfies Parameters<typeof meFixture>[0];

export default {
  title: "Settings/Settings home",
  component: SettingsHome,
} satisfies Meta<typeof SettingsHome>;

type Story = StoryObj<typeof SettingsHome>;

export const Ready: Story = { render: home(LEAD) };

export const ReadyDark: Story = {
  globals: { theme: "dark" },
  render: home(LEAD),
};

// The floor: a seat holding no object grant at all. The personal pages are the
// ones no grant gates, so they stay — and both conditional panels are dropped
// rather than drawn empty, which is what a heading over nothing would be.
export const PersonalOnly: Story = {
  render: home({ roles: ["rep"], seat: "full", rowScope: "own" }),
};

// The page exists and this seat may not open it. It says so without naming the
// grant that is missing: the address stays in the bar for the reader to quote,
// and naming the object would tell somebody who cannot open the page exactly
// what to ask to be given.
export const AddressNotYours: Story = { render: boundary("denied") };

// Nothing answers this address at all — an older link, or a typo. A different
// fact from the one above, and told as one: "not found" to a page that exists is
// a lie the reader can disprove by asking a colleague.
export const AddressUnknown: Story = { render: boundary("unknown") };

// Both boundaries in dark, because the empty state is a framed ground with one
// link in it and the frame is a derived value.
export const AddressUnknownDark: Story = {
  globals: { theme: "dark" },
  render: boundary("unknown"),
};
