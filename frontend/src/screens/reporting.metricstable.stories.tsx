// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { MetricsTable } from "./reporting.metricstable";
import { reportingStoryEvaluation } from "./reporting.story-fixtures";
import { StoryProviders } from "./story-utils";

const meta: Meta = {
  title: "Records/Reports/Analytics/All metrics",
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj;

// A metric's name opens its records; the reference it asks for is echoed below.
function Preview() {
  const [reference, select] = useState("");
  return (
    <StoryProviders>
      <MetricsTable
        evaluation={reportingStoryEvaluation}
        onEvidence={(reference) => select(JSON.stringify(reference))}
      />
      <p aria-live="polite">{reference}</p>
    </StoryProviders>
  );
}

export const Default: Story = { render: () => <Preview /> };
