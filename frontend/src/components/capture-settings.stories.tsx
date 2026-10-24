// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  permissionDenied,
  StoryProviders,
} from "../testing/story-utils";
import { CaptureSettingsCard } from "./capture-settings";

// The denied fixture holds `capture_settings` read without update: `meRoute({})`
// is an admin with no object grants, which draws a different card.
function story(
  auto_enrich: boolean,
  allow: Parameters<typeof meRoute>[0],
  refuseWrites = false,
  locks: unknown[] = [],
) {
  return () => {
    installFetchStub({
      "GET /me": meRoute(allow),
      ...(refuseWrites && { "PATCH /capture/settings": permissionDenied }),
      "GET /capture/settings": () =>
        jsonResponse({
          auto_enrich,
          mail_sharing: true,
          shared_posture_allowed: false,
          signature_enrich: true,
          auto_enrich_daily_cap: 500,
          site_read: { max_pages: 60, max_mib: 32, wall_seconds: 240 },
          mail_sync_interval_seconds: 120,
          confidentiality_pass_cap: 40,
          locks,
        }),
    });
    return (
      <StoryProviders>
        <CaptureSettingsCard />
      </StoryProviders>
    );
  };
}

const MANAGER = { capture_settings: ["read", "update"] } as const;
const READER = { capture_settings: ["read"] } as const;

const meta = {
  title: "Settings/Data/Capture rules/Enrichment and website reads",
  component: CaptureSettingsCard,
} satisfies Meta<typeof CaptureSettingsCard>;
export default meta;
type Story = StoryObj<typeof meta>;

export const EnrichOn: Story = { render: story(true, MANAGER) };

export const EnrichOff: Story = { render: story(false, MANAGER) };

// The state the seeded demo cannot show: the switch keeps the setting readable,
// refuses the change, and says why — with the explanation tied to the control by
// `aria-describedby` rather than sitting beside it. This is the one
// accessibility-wired denial in the tree, so it is worth a picture.
export const CannotChange: Story = { render: story(true, READER) };

// The denial in dark, next to `CannotChange` for comparison. A refused switch
// says so twice: the sentence beneath it, and the track — which is desaturated
// rather than dimmed, because the on-state has to stay legible. Desaturation is
// the fragile half of that: the dark palette's live green sits about where the
// light palette's muted one does, so this is the story that shows whether a
// reader in dark can still tell a refused switch from an operable one before
// clicking it.
export const CannotChangeDark: Story = {
  globals: { theme: "dark" },
  render: story(true, READER),
};

// A limit the environment sets: refused for an administrator too, with the
// variable to change in its place, while the other rows stay editable.
export const LockedByEnvironment: Story = {
  render: story(true, MANAGER, false, [
    {
      key: "capture.auto_enrich_daily_cap",
      origin: { source: "env", name: "MARGINCE_CAPTURE_AUTO_ENRICH_DAILY_CAP" },
    },
  ]),
};

// Six rows at 390px: each switch and number box drops under its words.
export const EnrichOnPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(true, MANAGER),
};

export const LimitRefused: Story = {
  render: story(true, MANAGER, true),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const cap = await canvas.findByTestId("capture-daily-cap");
    await userEvent.clear(cap);
    await userEvent.type(cap, "5000{Enter}");
    await canvas.findByText("Setting not changed");
    await expect(cap).toHaveAccessibleDescription(
      expect.stringContaining("Setting not changed"),
    );
    await expect(
      canvas.getByTestId("capture-auto-enrich-toggle"),
    ).not.toHaveAccessibleDescription(
      expect.stringContaining("Setting not changed"),
    );
    await expect(
      canvas.getByTestId("capture-read-max-pages"),
    ).not.toHaveAccessibleDescription(
      expect.stringContaining("Setting not changed"),
    );
  },
};
