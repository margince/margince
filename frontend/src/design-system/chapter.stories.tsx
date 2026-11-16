// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider } from "../i18n";
import { Badge, Button } from "./atoms";
import { Chapter, ChapterJump, Chapters } from "./chapter";
import { Panel, PanelBody } from "./panel";

// A long record tab as chapters: the same anatomy in each, so the reader
// learns it once. The jump chips lead to each chapter.
const meta: Meta<typeof Chapter> = {
  title: "Components/Layout and structure/Chapter",
  component: Chapter,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <div style={{ maxWidth: "48rem" }}>
          <Story />
        </div>
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof Chapter>;

function Tab({ rows = false }: Readonly<{ rows?: boolean }>) {
  return (
    <Chapters>
      <ChapterJump
        label="On this tab"
        chapters={[
          { id: "way", title: "Way in", reading: "3 routes" },
          { id: "record", title: "Record keeping" },
        ]}
      />
      <Chapter
        id="way"
        title="Way in"
        answer="3 routes: 2 direct · 1 via a contact."
        action={<Button>Record an introduction</Button>}
      >
        <Panel title="Ask Sofia Meier. They already correspond with this contact.">
          <PanelBody>
            <p>14 two-way exchanges in 90 days · last contact 4 days ago</p>
          </PanelBody>
        </Panel>
        <Panel title="Other routes">
          <PanelBody>
            <p>Martin Weber · weak</p>
          </PanelBody>
        </Panel>
      </Chapter>
      <Chapter
        id="record"
        title="Record keeping"
        answer="The fields, permissions and settings kept on this contact."
        rows={rows}
      >
        <Panel title="Communication permissions">
          <PanelBody>
            <Badge tone="success">Recent business contact recorded</Badge>
          </PanelBody>
        </Panel>
        <Panel title="Lists">
          <PanelBody>
            <Badge>Q4 fleet push</Badge>
          </PanelBody>
        </Panel>
      </Chapter>
    </Chapters>
  );
}

export const Stacked: Story = { render: () => <Tab /> };

// Small, unlike sections read as rows: the name on the left, the body beside.
export const Rows: Story = { render: () => <Tab rows /> };

// A chapter whose answer is the whole reading draws no empty card.
export const AnswerOnly: Story = {
  render: () => (
    <Chapter
      id="none"
      title="Way in"
      answer="No one corresponds with this contact"
    />
  ),
};
