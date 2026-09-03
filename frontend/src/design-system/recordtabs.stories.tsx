// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { LocaleProvider } from "../i18n";
import { Button } from "./atoms";
import { RecordTabs } from "./recordtabs";

// RecordTabs: the strip that chooses which body of a record is open, drawn as
// a rule with the current tab underlined rather than as a pill. Three things
// beside the ordinary counted row are worth seeing on their own: a tab that is
// not a list of things and so carries no count at all, and a tab carrying the
// unread mark — something waiting behind it that nobody has taken up.
const meta: Meta<typeof RecordTabs> = {
  title: "Design System/RecordTabs",
  component: RecordTabs,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;
type Story = StoryObj<typeof RecordTabs>;

type Body = "overview" | "people" | "activity" | "brief";

const LABELS: Record<Body, string> = {
  overview: "Overview",
  people: "People",
  activity: "Activity",
  brief: "Brief",
};

// Live, so the underline can be judged moving to the tab the reader picked.
function Live(props: Readonly<{ initial: Body }>) {
  const [value, setValue] = useState<Body>(props.initial);
  return (
    <RecordTabs
      label="Record"
      options={["overview", "people", "activity", "brief"]}
      value={value}
      onChange={setValue}
      labels={LABELS}
      counts={{ people: 10, activity: 27 }}
      marks={{ brief: true }}
    />
  );
}

// The ordinary row: two tabs counted, `overview` and `brief` carrying none —
// `overview` because it is a summary rather than a list, `brief` because it is
// a document. `brief` also carries the unread mark, so the dot and a missing
// count sit on the same tab without being confused for each other.
export const Mixed: Story = {
  render: () => <Live initial="overview" />,
};

// The tab with no count is the current one — being selected does not require a
// figure any more than it requires one on the row above.
export const SelectedTabHasNoCount: Story = {
  render: () => <Live initial="brief" />,
};

// The row's far end carries the control that opens the record's details
// column. It sits off the strip's own scroll, so it stays in view however
// many tabs there are, and reads as the page's control rather than a tab.
export const WithTrailingControl: Story = {
  render: () => (
    <RecordTabs
      label="Record"
      options={["overview", "people", "activity", "brief"]}
      value="overview"
      onChange={() => undefined}
      labels={LABELS}
      counts={{ people: 10, activity: 27 }}
      trailing={
        <Button small aria-pressed={false}>
          Details
        </Button>
      }
    />
  ),
};
