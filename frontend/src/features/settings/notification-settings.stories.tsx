// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { PageSaveBar, SavePageProvider } from "../../design-system/savepage";
import { SettingList } from "../../design-system/settingrow";
import type { BrowserNotificationPermission } from "../../lib/notificationpermission";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import {
  BrowserNotificationsRow,
  NotificationSettingsCard,
} from "./notification-settings";

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

function story(items: Preference[], refuseWrites = false) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /me/notification-preferences": () => jsonResponse({ items }),
      ...(refuseWrites && {
        "PUT /me/notification-preferences": () =>
          jsonResponse(
            {
              status: 422,
              code: "validation_error",
              detail: "That delivery is not offered for this kind.",
            },
            422,
          ),
      }),
    });
    return (
      <StoryProviders>
        {/* A pick waits on the page's one save bar, as on Settings. */}
        <SavePageProvider>
          <NotificationSettingsCard />
          <PageSaveBar />
        </SavePageProvider>
      </StoryProviders>
    );
  };
}

const meta: Meta<typeof NotificationSettingsCard> = {
  title: "Settings/You/Notifications/Delivery by kind",
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
//
// Every pair here is one the server would actually answer with: `email` is the
// approval class's alone, so the batch is what a lead-SLA row reaches a mailbox
// by. A fixture holding a combination the product refuses would draw a control
// sitting on a value it does not offer, which is a face no reader can ever get
// to and the wrong thing to review a screen against.
export const DecidedBesideDefaults: Story = {
  render: story([
    preference("approval_pending", "in_app", true),
    preference("automation", "off", true),
    preference("lead_sla", "digest", true),
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

export const StandingDefaultsPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story(standingDefaults()),
};

export const SaveRefused: Story = {
  render: story(standingDefaults(), true),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const body = within(canvasElement.ownerDocument.body);
    const refused = await canvas.findByRole("combobox", {
      name: /Automations that ran/,
    });
    await userEvent.click(refused);
    await userEvent.click(await body.findByRole("option", { name: "Off" }));
    const alert = await canvas.findByRole("alert");
    await expect(refused.getAttribute("aria-describedby")).toContain(alert.id);
    await expect(
      canvas
        .getByRole("combobox", { name: /Approvals waiting on you/ })
        .getAttribute("aria-describedby"),
    ).not.toContain(alert.id);
  },
};

// The browser notifications row in each answer a browser gives. Only the
// undecided browser offers Allow; the others say where the switch is.
function browserRow(permission: BrowserNotificationPermission) {
  return () => (
    <StoryProviders>
      <SettingList bleed="settings">
        <BrowserNotificationsRow permission={permission} onAllow={() => {}} />
      </SettingList>
    </StoryProviders>
  );
}

export const BrowserNotificationsUndecided: Story = {
  render: browserRow("default"),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("button", { name: "Allow" }),
    ).toBeVisible();
  },
};

export const BrowserNotificationsAllowed: Story = {
  render: browserRow("granted"),
};

export const BrowserNotificationsBlocked: Story = {
  render: browserRow("denied"),
};
