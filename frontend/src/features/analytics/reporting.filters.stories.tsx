import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { installFetchStub, StoryProviders } from "../../testing/story-utils";
import { ReportingFilters } from "./reporting.filters";
import {
  reportingStoryEvaluation,
  reportingStoryRoutes,
} from "./reporting.story-fixtures";

const meta: Meta = { title: "Records/Reports/Analytics/Report filters" };
export default meta;
function Preview() {
  const [selection, setSelection] = useState(
    reportingStoryEvaluation.selection,
  );
  return <ReportingFilters selection={selection} onChange={setSelection} />;
}
export const Editable: StoryObj = {
  render: () => {
    installFetchStub(reportingStoryRoutes());
    return (
      <StoryProviders>
        <Preview />
      </StoryProviders>
    );
  },
};
