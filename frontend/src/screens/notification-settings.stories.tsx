// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { NotificationSettingsCard } from "./notification-settings";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";

// Where each class of notice reaches this seat. There is no permission story
// here: both endpoints read and write the reader's own rows, so no seat can be
// shown the dropdowns refused — which is why the fixtures below vary the
// CHOICES instead.

type Preference = { class: string; delivery: string; chosen: boolean };

function preference(
  notificationClass: string,
  delivery: string,
  chosen: boolean,
): Preference {
  return { class: notificationClass, delivery, chosen };
}

// The whole set, as the server always answers it, with the compiled defaults
// standing: approvals by email and everything else in the app.
function standingDefaults(): Preference[] {
  return [
    preference("approval_pending", "email", false),
    preference("automation", "in_app", false),
    preference("lead_sla", "in_app", false),
    preference("capture", "in_app", false),
    preference("system", "in_app", false),
    preference("coach", "in_app", false),
  ];
}

function story(items: Preference[]) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /me/notification-preferences": () => jsonResponse({ items }),
    });
    return (
      <StoryProviders>
        <NotificationSettingsCard />
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof NotificationSettingsCard> = {
  title: "Settings/You/Notifications/How each kind reaches you",
  component: NotificationSettingsCard,
};
export default meta;
type Story = StoryObj<typeof NotificationSettingsCard>;

// First visit: nothing decided, every row sitting on the standing default.
export const StandingDefaults: Story = {
  render: story(standingDefaults()),
};

// Decisions the seat has made, beside classes still following the default. The
// two are drawn the same on purpose — what the row shows is what happens today,
// and a decided row is not a louder fact than an undecided one.
export const DecidedBesideDefaults: Story = {
  render: story([
    preference("approval_pending", "in_app", true),
    preference("automation", "off", true),
    preference("lead_sla", "email", true),
    preference("capture", "in_app", false),
    preference("system", "in_app", false),
    preference("coach", "digest", true),
  ]),
};

// A seat that has muted everything it may mute. Coaching is the exception and
// stays on a delivery, because the server refuses `off` for it — the row that
// cannot be switched off reads as an ordinary row with one fewer option.
export const AsQuietAsItGets: Story = {
  render: story([
    preference("approval_pending", "off", true),
    preference("automation", "off", true),
    preference("lead_sla", "off", true),
    preference("capture", "off", true),
    preference("system", "off", true),
    preference("coach", "in_app", false),
  ]),
};

// Six dropdown faces down one column, in dark. The closed face is this card's
// only ink beside the labels, so a wrong control token shows here first.
export const StandingDefaultsDark: Story = {
  globals: { theme: "dark" },
  render: story(standingDefaults()),
};
