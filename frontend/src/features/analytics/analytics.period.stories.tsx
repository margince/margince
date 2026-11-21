// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useLocale, useT } from "../../i18n";
import { StoryProviders } from "../../testing/story-utils";
import { periodCaption } from "./analytics.period";
import type { ReportingEvaluation } from "./reporting.model";
import { reportingStoryEvaluation } from "./reporting.story-fixtures";

// The caption under a reading: the whole period, and the cutoff its actuals run
// through on the clock that counted them.

const meta: Meta = { title: "Records/Reports/Analytics/Period caption" };
export default meta;
type Story = StoryObj;

const { context } = reportingStoryEvaluation;

function CaptionLine({
  evaluation,
}: Readonly<{ evaluation: ReportingEvaluation }>) {
  const t = useT();
  const { locale } = useLocale();
  return <p className="t-caption">{periodCaption(evaluation, locale, t)}</p>;
}

function Caption({
  evaluation,
}: Readonly<{ evaluation: ReportingEvaluation }>) {
  return (
    <StoryProviders>
      <CaptionLine evaluation={evaluation} />
    </StoryProviders>
  );
}

/** A running period: its whole span, then the cutoff and its zone. */
export const Running: Story = {
  render: () => <Caption evaluation={reportingStoryEvaluation} />,
};

/** A period that has not started names its first instant and zone. */
export const NotStarted: Story = {
  render: () => (
    <Caption
      evaluation={{
        ...reportingStoryEvaluation,
        context: {
          ...context,
          interval: {
            start_at: context.interval.end_at,
            end_at: context.interval.end_at,
          },
        },
      }}
    />
  ),
};

/** A closed period names its span and no cutoff. */
export const Closed: Story = {
  render: () => (
    <Caption
      evaluation={{
        ...reportingStoryEvaluation,
        context: {
          ...context,
          horizon: undefined,
          evaluated_at: "2026-10-05T09:00:00Z",
        },
      }}
    />
  ),
};
