import type { Meta, StoryObj } from "@storybook/react-vite";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { AnalyticsScreen } from "./analytics";
import {
  ADMIN_GRANTS,
  metric,
  overviewRoutes,
  prospectingEvaluation,
  salesEvaluation,
} from "./analytics.overview.testkit";
import type { ReportingEvaluation } from "./reporting.model";

// Analytics' opening page as each reader meets it. The address names the view,
// so each story opens the one it is about whatever a reader chose before.
function overview(
  view: "sales" | "prospecting",
  evaluation: ReportingEvaluation | (() => Response),
  routes: Parameters<typeof overviewRoutes>[0] = {},
) {
  return () => {
    globalThis.location.hash = `#/analytics/overview?overview=${view}`;
    installFetchStub({
      ...overviewRoutes(routes),
      "GET /analytics/evaluate":
        typeof evaluation === "function"
          ? evaluation
          : () => jsonResponse(evaluation),
    });
    return (
      <StoryProviders>
        <AnalyticsScreen />
      </StoryProviders>
    );
  };
}

const meta: Meta = {
  title: "Records/Reports/Analytics/Overview",
  parameters: { layout: "padded" },
  beforeEach: () => {
    const previousHash = globalThis.location.hash;
    return () => {
      globalThis.location.hash = previousHash;
    };
  },
};
export default meta;
type Story = StoryObj;

export const SalesReview: Story = {
  render: overview("sales", salesEvaluation()),
};

export const SalesReviewDark: Story = {
  render: overview("sales", salesEvaluation()),
  globals: { theme: "dark" },
};

export const SalesReviewOnAPhone: Story = {
  render: overview("sales", salesEvaluation()),
  globals: { viewport: { value: "phone" } },
};

export const SalesReviewWithoutATarget: Story = {
  render: overview(
    "sales",
    salesEvaluation({
      comparison: { status: "no_target", target_metric: "bookings_won" },
    }),
    { grants: ADMIN_GRANTS },
  ),
};

export const SalesReviewWithAFailedFigure: Story = {
  render: overview(
    "sales",
    salesEvaluation({
      metrics: [
        metric("bookings_won", null, {
          coverage: { status: "failed", withheld: false },
        }),
        metric("open_pipeline", 136_000_000),
      ],
    }),
    {
      checks: () => jsonResponse({ title: "Unavailable", status: 503 }, 503),
    },
  ),
};

export const MyProspecting: Story = {
  render: overview("prospecting", prospectingEvaluation(), { seat: "sdr" }),
};

export const MyProspectingDark: Story = {
  render: overview("prospecting", prospectingEvaluation(), { seat: "sdr" }),
  globals: { theme: "dark" },
};

export const MyProspectingOnAPhone: Story = {
  render: overview("prospecting", prospectingEvaluation(), { seat: "sdr" }),
  globals: { viewport: { value: "phone" } },
};

export const MyProspectingWithoutHandoffs: Story = {
  render: overview("prospecting", prospectingEvaluation(), {
    seat: "sdr",
    template: "sales",
  }),
};

export const MyProspectingWithNothingYet: Story = {
  render: overview(
    "prospecting",
    prospectingEvaluation([
      metric("meetings_held", null, {
        coverage: { status: "no_data", withheld: false },
      }),
      metric("accepted_opportunities", null, {
        coverage: { status: "no_data", withheld: false },
      }),
      metric("leads_promoted", null, {
        coverage: { status: "no_data", withheld: false },
      }),
      metric("lead_first_response", null, {
        coverage: { status: "insufficient_sample", withheld: false },
      }),
    ]),
    { seat: "sdr" },
  ),
};
