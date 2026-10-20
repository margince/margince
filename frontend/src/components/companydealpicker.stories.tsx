import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { CompanyDealPicker } from "./companydealpicker";

const meta = {
  title: "Components/Forms and input/Company deal picker",
  component: CompanyDealPicker,
} satisfies Meta<typeof CompanyDealPicker>;
export default meta;
type Story = StoryObj<typeof meta>;
function Picker() {
  const [picked, setPicked] = useState("");
  return (
    <CompanyDealPicker
      companyId="company-one"
      value={picked}
      onChange={setPicked}
    />
  );
}
export const ExistingDeals: Story = {
  args: { companyId: "company-one", value: "", onChange: () => {} },
  render: () => {
    installFetchStub({
      "GET /deals": () =>
        jsonResponse({
          data: [{ id: "support", name: "Support renewal" }],
          page: { has_more: false },
        }),
    });
    return (
      <StoryProviders>
        <Picker />
      </StoryProviders>
    );
  },
};
