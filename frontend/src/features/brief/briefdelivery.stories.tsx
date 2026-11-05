// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import { PageSaveBar, SavePageProvider } from "../../design-system/savepage";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  type RouteMap,
  StoryProviders,
} from "../../testing/story-utils";
import { BriefDeliveryPanel } from "./briefdelivery";

function story(delivery: Record<string, string>, routes: RouteMap = {}) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({}),
      "GET /teams": () => jsonResponse({ data: [], page: { has_more: false } }),
      "GET /me/brief-delivery": () => jsonResponse(delivery),
      ...routes,
    });
    return (
      <StoryProviders>
        <div className="settings-stack">
          {/* The panel drafts and the bar writes, so a story without the bar
              cannot show a save at all, refused or not. */}
          <SavePageProvider>
            <BriefDeliveryPanel />
            <PageSaveBar />
          </SavePageProvider>
        </div>
      </StoryProviders>
    );
  };
}

const meta = {
  title: "Settings/You/Notifications/Email summaries",
  component: BriefDeliveryPanel,
  parameters: { layout: "padded" },
} satisfies Meta<typeof BriefDeliveryPanel>;
export default meta;
type Story = StoryObj<typeof meta>;

// Never chosen: both rows sit on what happens today, which is by email.
export const NeverChosen: Story = { render: story({}) };

export const WeeklyOff: Story = {
  render: story({ morning_brief_delivery: "email", weekly_delivery: "none" }),
};

export const NeverChosenDark: Story = {
  globals: { theme: "dark" },
  render: story({}),
};

export const NeverChosenPhone: Story = {
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  render: story({}),
};

// A refused save is said under the rows, with the server's reason, and the rows
// it covered point at it.
export const SaveRefused: Story = {
  render: story(
    {},
    {
      "PUT /me/brief-delivery": () =>
        new Response(
          JSON.stringify({ status: 503, code: "service_unavailable" }),
          {
            status: 503,
            headers: { "content-type": "application/problem+json" },
          },
        ),
    },
  ),
  play: async ({ canvasElement }) => {
    const page = within(canvasElement.ownerDocument.body);
    const [morning] = await page.findAllByRole("combobox");
    if (!morning) throw new Error("no delivery choice drawn");
    await userEvent.click(morning);
    await userEvent.click(
      await page.findByRole("option", { name: "Not by email" }),
    );
    // The page writes through the save bar, so the choice alone drafts and
    // refuses nothing. The notice answers the write.
    await userEvent.click(await page.findByRole("button", { name: "Save" }));
    // Two notices answer one refusal: the bar summarises it, and the rows that
    // were in the write point at the one under the list.
    const alerts = await page.findAllByRole("alert");
    const describes = morning.getAttribute("aria-describedby") ?? "";
    if (!alerts.some((candidate) => describes.includes(candidate.id)))
      throw new Error("the refused row points at no notice");
  },
};

export const SaveRefusedPhone: Story = {
  ...SaveRefused,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
