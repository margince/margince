// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Panel, PanelBody } from "../../../design-system/panel";
import { StoryProviders } from "../../../testing/story-utils";
import { RecordReadingPair } from "./reading";

// The two reference sections a reader consults rather than reads, at half the
// measure each, folding to one column where two would not fit.
const meta: Meta<typeof RecordReadingPair> = {
  title: "Records/Record 360/Reading pair",
  component: RecordReadingPair,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof RecordReadingPair>;

function Reference({ title }: Readonly<{ title: string }>) {
  return (
    <Panel title={title}>
      <PanelBody>
        <p className="t-caption">
          A reference section a reader consults rather than reads.
        </p>
      </PanelBody>
    </Panel>
  );
}

export const TwoSections: Story = {
  render: () => (
    <StoryProviders>
      <div style={{ maxWidth: 860 }}>
        <RecordReadingPair>
          <Reference title="What was said lately" />
          <Reference title="Commitments" />
        </RecordReadingPair>
      </div>
    </StoryProviders>
  ),
};
