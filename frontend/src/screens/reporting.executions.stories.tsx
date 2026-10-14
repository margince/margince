import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../testing/story-utils";
import { ReportingExecutions } from "./reporting.executions";
import { reportingExecutions } from "./reporting.scenarios";

const meta: Meta = { title: "Records/Reports/Analytics/Executions" };
export default meta;
type Story = StoryObj;
export const Failures: Story = {
  render: () => {
    installFetchStub({
      "GET /analytics/reports/report/executions": () =>
        jsonResponse({ data: reportingExecutions }),
    });
    return (
      <StoryProviders>
        <ReportingExecutions reportId="report" canRetry />
      </StoryProviders>
    );
  },
};
