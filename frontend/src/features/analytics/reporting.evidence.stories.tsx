import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, waitFor, within } from "storybook/test";
import { formatDate } from "../../format/format.dates";
import { UTC_ZONE } from "../../format/timezone";
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

// A meeting known only by its date shows that date and no time: its stored noon
// is no hour anyone said.
export const DayMeeting: Story = {
  render: () => {
    installFetchStub({
      ...reportingStoryRoutes(),
      "GET /analytics/evidence": () =>
        jsonResponse({
          context: reportingStoryEvaluation.context,
          metric: "meetings_held",
          rows: [
            {
              key: "meeting",
              source_type: "activity",
              source_id: "00000000-0000-4000-8000-000000000031",
              label: "Trade fair",
              value: 1,
              occurred_at: "2026-09-06T12:00:00Z",
              occurred_precision: "day",
              restricted: false,
            },
          ],
          truncated: false,
        }),
    });
    return (
      <StoryProviders>
        <ReportingEvidenceDrawer
          evaluation={reportingStoryEvaluation}
          reference={{ metric: "meetings_held", context_id: "interval" }}
          onClose={() => {}}
        />
      </StoryProviders>
    );
  },
  play: async () => {
    const dialog = within(await within(document.body).findByRole("dialog"));
    const cell = await dialog.findByText(
      formatDate("2026-09-06T12:00:00Z", "en", UTC_ZONE),
    );
    await expect(cell.textContent).not.toMatch(/\d:\d/);
  },
};

// The drawer for one figure in a given coverage state, against the routes
// that would list its records if it asked.
function FigureIn({
  coverage,
  value,
}: Readonly<{
  coverage: (typeof reportingStoryEvaluation.metrics)[number]["coverage"];
  value: number | null;
}>) {
  installFetchStub({
    ...reportingStoryRoutes(),
    "GET /analytics/evidence": () =>
      jsonResponse({
        context: reportingStoryEvaluation.context,
        metric: "bookings_won",
        rows: [],
        truncated: false,
      }),
  });
  return (
    <StoryProviders>
      <ReportingEvidenceDrawer
        evaluation={{
          ...reportingStoryEvaluation,
          metrics: reportingStoryEvaluation.metrics.map((metric) =>
            metric.id === "bookings_won"
              ? { ...metric, value, coverage }
              : metric,
          ),
        }}
        reference={{ metric: "bookings_won", context_id: "interval" }}
        onClose={() => {}}
      />
    </StoryProviders>
  );
}

// A measured zero: its explanation, then its empty permitted population.
export const MeasuredZero: Story = {
  render: () => (
    <FigureIn coverage={{ status: "no_data", withheld: false }} value={0} />
  ),
  play: async () => {
    const dialog = within(await within(document.body).findByRole("dialog"));
    await dialog.findByText(en["reporting.evidenceZero"]);
  },
};

// A figure that failed lists nothing and offers to read it again.
export const FailedFigure: Story = {
  render: () => (
    <FigureIn coverage={{ status: "failed", withheld: false }} value={null} />
  ),
  play: async () => {
    const dialog = within(await within(document.body).findByRole("dialog"));
    await dialog.findByText(en["reporting.evidenceUnavailable"]);
    await expect(
      dialog.getByRole("button", { name: en["common.retry"] }),
    ).toBeInTheDocument();
  },
};

// A figure hidden from the reader shows neither its records nor their count.
export const HiddenFigure: Story = {
  render: () => (
    <FigureIn coverage={{ status: "ok", withheld: true }} value={null} />
  ),
  play: async () => {
    const dialog = within(await within(document.body).findByRole("dialog"));
    await dialog.findByText(en["reporting.evidenceWithheld"]);
    await expect(dialog.queryByRole("table")).toBeNull();
  },
};
