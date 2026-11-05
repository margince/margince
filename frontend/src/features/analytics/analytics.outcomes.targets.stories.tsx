import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  meRoute,
  StoryProviders,
} from "../../testing/story-utils";
import { MyTargets } from "./analytics.outcomes.targets";
import {
  reportingStoryRoutes,
  routeSeatEvaluation,
  type SeatFigure,
} from "./reporting.story-fixtures";

const meta: Meta = { title: "Records/Reports/Own targets" };
export default meta;
type Story = StoryObj;

function seat(figures: Readonly<Record<string, SeatFigure>>) {
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /me": meRoute({
      report_definition: ["read"],
      reporting_framework: ["read"],
      sales_target: ["read"],
    }),
  });
  routeSeatEvaluation(figures);
  return (
    <StoryProviders>
      <MyTargets self="u-rep-1" locale="en" />
    </StoryProviders>
  );
}

// A quarter target on meetings, a month target on sales, and the rest unset.
export const Default: Story = {
  render: () =>
    seat({
      meetings_held: { actual: 10, targets: { fiscal_quarter: 15 } },
      bookings_won: {
        actual: 21_600_000,
        targets: { month: 30_000_000, fiscal_quarter: 90_000_000 },
      },
      accepted_opportunities: { actual: 3 },
      qualified_pipeline_created: { actual: null },
    }),
};

// A seat past its target: the meter is full and the share goes over 100.
export const Exceeded: Story = {
  render: () =>
    seat({
      meetings_held: { actual: 18, targets: { month: 15 } },
    }),
};

// No target for any figure: each card says so and keeps the month's figure.
export const NoTargets: Story = {
  render: () => seat({ bookings_won: { actual: 2_500_000 } }),
};

// A figure the seat may not see in full: no target is applied to it.
export const Withheld: Story = {
  render: () =>
    seat({
      meetings_held: { actual: 4, targets: { month: 15 }, withheld: true },
    }),
};

export const Phone: Story = {
  ...Default,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
