import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { DimensionFilters } from "./reporting.dimensions";
import {
  reportingStoryEvaluation,
  reportingStoryRoutes,
} from "./reporting.story-fixtures";

const meta: Meta = {
  title: "Records/Reports/Analytics/Dimension filters",
  beforeEach: () =>
    installFetchStub({
      ...reportingStoryRoutes(),
      "GET /products": () => jsonResponse({ data: [] }),
      "GET /analytics/initiatives": () => jsonResponse({ data: [] }),
    }),
};
export default meta;
type Story = StoryObj;
function Filters() {
  const [selection, setSelection] = useState(
    reportingStoryEvaluation.selection,
  );
  return <DimensionFilters selection={selection} onChange={setSelection} />;
}
export const Default: Story = {
  render: () => (
    <StoryProviders>
      <Filters />
    </StoryProviders>
  ),
};
export const Dark: Story = { ...Default, globals: { theme: "dark" } };
