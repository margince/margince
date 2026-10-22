import type { Meta, StoryObj } from "@storybook/react-vite";
import { suggestion } from "../testing/dealsuggestion.fixture";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { DealSuggestionAnswerDialog } from "./dealsuggestion.answer";

const meta = {
  title: "Records/Deals/Suggestion answer",
  component: DealSuggestionAnswerDialog,
} satisfies Meta<typeof DealSuggestionAnswerDialog>;
export default meta;
type Story = StoryObj<typeof meta>;

export const LinkExisting: Story = {
  args: { suggestion, action: "link", onClose: () => {} },
  render: (args) => {
    installFetchStub({
      "GET /deals": () =>
        jsonResponse({
          data: [{ id: "deal-support", name: "Support renewal" }],
          page: { has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <DealSuggestionAnswerDialog {...args} />
      </StoryProviders>
    );
  },
};
export const DismissWithReason: Story = {
  args: { suggestion, action: "dismiss", onClose: () => {} },
  render: (args) => (
    <StoryProviders>
      <DealSuggestionAnswerDialog {...args} />
    </StoryProviders>
  ),
};
export const NoExistingDeals: Story = {
  ...LinkExisting,
  render: (args) => {
    installFetchStub({
      "GET /deals": () => jsonResponse({ data: [], page: { has_more: false } }),
    });
    return (
      <StoryProviders>
        <DealSuggestionAnswerDialog {...args} />
      </StoryProviders>
    );
  },
};
