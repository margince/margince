import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { en } from "../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "../testing/story-utils";
import { useLogDealFiling } from "./logactivity.dealfiling";

const meta: Meta = {
  title: "Patterns/Log activity deal filing",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;
type Options = components["schemas"]["ActivityDealFilingOptions"];

function FilingField() {
  return useLogDealFiling("contact", "p1", true).field;
}

function filingStory(options: Options) {
  return () => {
    installFetchStub({
      "GET /me": meRoute({
        activity: ["read", "create"],
        contact: ["read"],
        company: ["read"],
        deal: ["read"],
      }),
      "GET /activities/deal-filing": () => jsonResponse(options),
      "GET /deals/d1": () =>
        jsonResponse({ id: "d1", name: "Account renewal" }),
      "GET /deals": () =>
        jsonResponse({
          data: [
            { id: "d1", name: "Account renewal" },
            { id: "d2", name: "New site" },
          ],
          page: { has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <FilingField />
      </StoryProviders>
    );
  };
}

export const OneOpenDeal: Story = {
  render: filingStory({
    company_id: "o1",
    suggested_deal_id: "d1",
    choice_required: false,
  }),
};
export const OneOpenDealDark: Story = {
  ...OneOpenDeal,
  globals: { theme: "dark" },
};
export const SeveralOpenDeals: Story = {
  render: filingStory({ company_id: "o1", choice_required: true }),
};
export const SeveralOpenDealsDark: Story = {
  ...SeveralOpenDeals,
  globals: { theme: "dark" },
};
export const ExplicitNone: Story = {
  ...SeveralOpenDeals,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: en["log.dealNone"] }),
    );
    await expect(
      canvas.getByRole("button", { name: en["log.dealNone"] }),
    ).toHaveAttribute("aria-pressed", "true");
  },
};
export const ExplicitNoneDark: Story = {
  ...ExplicitNone,
  globals: { theme: "dark" },
};
