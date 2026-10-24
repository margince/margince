import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, waitFor, within } from "storybook/test";
import { en } from "../../i18n/en";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { ReportingEvidenceDrawer } from "./reporting.evidence";
import {
  reportingLeadsEvaluation,
  reportingSectionRoutes,
  reportingStoryEvaluation,
  reportingStoryRoutes,
} from "./reporting.story-fixtures";

const meta: Meta = { title: "Records/Reports/Analytics/Evidence" };
export default meta;
type Story = StoryObj;
function Preview({
  earlierDefinition = false,
}: Readonly<{ earlierDefinition?: boolean }>) {
  const [open, setOpen] = useState(true);
  return open ? (
    <ReportingEvidenceDrawer
      evaluation={
        earlierDefinition
          ? {
              ...reportingStoryEvaluation,
              metrics: reportingStoryEvaluation.metrics.map((metric) => ({
                ...metric,
                version: "1",
              })),
            }
          : reportingStoryEvaluation
      }
      editionId={earlierDefinition ? "old" : undefined}
      reference={{ metric: "bookings_won", context_id: "interval" }}
      onClose={() => setOpen(false)}
    />
  ) : (
    <p>Dialog closed</p>
  );
}
export const Default: Story = {
  render: () => {
    installFetchStub(reportingStoryRoutes());
    return (
      <StoryProviders>
        <Preview />
      </StoryProviders>
    );
  },
};

export const EarlierDefinition: Story = {
  render: () => {
    const routes = reportingStoryRoutes();
    const evidence = routes["GET /analytics/evidence"];
    if (!evidence) throw new Error("the reporting fixtures serve no evidence");
    installFetchStub({
      ...routes,
      "GET /analytics/editions/old/evidence": evidence,
    });
    return (
      <StoryProviders>
        <Preview earlierDefinition />
      </StoryProviders>
    );
  },
};

// The figures moved after the report loaded: the drawer says so and offers the
// reload, never the record sentence a lost edit gets.
export const StaleReading: Story = {
  render: () => {
    installFetchStub({
      ...reportingStoryRoutes(),
      "GET /analytics/evidence": () =>
        jsonResponse(
          { status: 409, code: "version_skew", title: "Conflict" },
          409,
        ),
    });
    return (
      <StoryProviders>
        <Preview />
      </StoryProviders>
    );
  },
  play: async () => {
    const dialog = within(await within(document.body).findByRole("dialog"));
    const stale = await dialog.findByText(en["reporting.evidenceStale"]);
    const retry = dialog.getByRole("button", { name: en["common.retry"] });
    // The drawer fades in from opacity 0, so visibility is waited for.
    await waitFor(() => expect(stale).toBeVisible());
    await expect(retry).toBeVisible();
  },
};

// One lead origin's records, opened from its bar.
export const LeadGroup: Story = {
  render: () => {
    installFetchStub(reportingSectionRoutes(reportingLeadsEvaluation));
    return (
      <StoryProviders>
        <ReportingEvidenceDrawer
          evaluation={reportingLeadsEvaluation}
          reference={{
            metric: "leads_created",
            context_id: "interval",
            group_key: "origin:web",
          }}
          onClose={() => {}}
        />
      </StoryProviders>
    );
  },
};
