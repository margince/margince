// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { ReactNode } from "react";
import { LocaleProvider } from "../i18n";
import { ProjectHealth } from "./projecthealth";
import type { ProjectHealthAssessment } from "./projecthealth.queries";
import { projectHealthKey } from "./projecthealth.queries";
import { installFetchStub, jsonResponse } from "./story-utils";

// How a delivery is going, and how it has been going.
//
// The state worth seeing beyond the ordinary one is the EMPTY case: it says
// nobody has judged the project, never "on track". A delivery assessed and
// found healthy and one nobody has looked at are different states, and a card
// that rendered them alike would hide exactly the projects going unwatched.
//
// The corrected reading is the other one: it stays in the history and is
// marked, because a correction is part of the record and a history with the
// mistake removed reads as though nobody ever got it wrong.

const PROJECT = "33333333-3333-3333-3333-333333333333";

const reading = (
  over: Partial<ProjectHealthAssessment>,
): ProjectHealthAssessment =>
  ({
    id: "h1",
    project_id: PROJECT,
    state: "on_track",
    assessed_at: "2026-09-08T09:00:00Z",
    source: "human",
    superseded: false,
    created_at: "2026-09-08T09:00:00Z",
    ...over,
  }) as ProjectHealthAssessment;

function Served({
  rows,
  children,
}: Readonly<{ rows: ProjectHealthAssessment[]; children: ReactNode }>) {
  const body = { data: rows, page: {} };
  installFetchStub({
    [`GET /projects/${PROJECT}/health-assessments`]: () => jsonResponse(body),
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  client.setQueryData(projectHealthKey(PROJECT), rows);
  return (
    <QueryClientProvider client={client}>
      <LocaleProvider initial="en">{children}</LocaleProvider>
    </QueryClientProvider>
  );
}

const meta: Meta<typeof ProjectHealth> = {
  title: "Records/Project/Delivery health",
  component: ProjectHealth,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ProjectHealth>;

/** A delivery that slipped, with the healthier readings behind it. */
export const AtRisk: Story = {
  render: () => (
    <Served
      rows={[
        reading({
          id: "h3",
          state: "at_risk",
          note: "Integration testing slipped a week; the client's environment is not ready.",
          assessed_at: "2026-09-08T09:00:00Z",
        }),
        reading({ id: "h2", assessed_at: "2026-09-01T09:00:00Z" }),
        reading({ id: "h1", assessed_at: "2026-08-25T09:00:00Z" }),
      ]}
    >
      <ProjectHealth projectId={PROJECT} />
    </Served>
  ),
};

/** A withdrawn reading beside the correction that replaced it. */
export const WithACorrection: Story = {
  render: () => (
    <Served
      rows={[
        reading({
          id: "h2",
          state: "off_track",
          note: "Corrected: the milestone was missed, not merely at risk.",
          supersedes_assessment_id: "h1",
        }),
        reading({
          id: "h1",
          state: "at_risk",
          note: "Milestone looks tight.",
          superseded: true,
          superseded_by_id: "h2",
        }),
      ]}
    >
      <ProjectHealth projectId={PROJECT} />
    </Served>
  ),
};

/** Nobody has looked. The detail line says what puts a reading here. */
export const NeverAssessed: Story = {
  render: () => (
    <Served rows={[]}>
      <ProjectHealth projectId={PROJECT} />
    </Served>
  ),
};
