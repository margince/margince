// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { EmptyState } from "../design-system/atoms";
import { useT } from "../i18n";
import { LinkedCaseNotice } from "./privacy.caselink.notice";
import { StoryProviders } from "./story-utils";

// What the privacy queue says when the link an officer followed names a case
// the page cannot show.
//
// ONE arm draws. `absent` is the notice; `none`, `loading` and `shown` all
// return null on purpose — a case still arriving is not a missing one, and a
// reader can disprove a notice that says otherwise by pressing Load more. So
// there is no story for them: a story of a component that renders nothing
// pictures nothing, and naming the quiet arms here would suggest there is a
// state this file failed to draw.

const ABSENT = { kind: "absent", id: "dsr-9" } as const;

const meta: Meta<typeof LinkedCaseNotice> = {
  title: "Settings/Governance/Privacy & retention/Linked case notice",
  component: LinkedCaseNotice,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof LinkedCaseNotice>;

/** Above the queue's rows: the link named a case none of the loaded pages has. */
export const CaseNotHere: Story = {
  render: () => (
    <StoryProviders>
      <LinkedCaseNotice linked={ABSENT} />
    </StoryProviders>
  ),
};

// The pairing the notice exists for, and the one a reader meets after
// narrowing the facet: "nothing here" alone answers a question the officer
// never asked — they came to read one case, not to learn whether this filter
// has rows. Same order the queue draws them in, notice first.
function EmptiedQueue() {
  const t = useT();
  return (
    <>
      <LinkedCaseNotice linked={ABSENT} />
      <EmptyState>{t("common.empty")}</EmptyState>
    </>
  );
}

export const AboveAnEmptyQueue: Story = {
  render: () => (
    <StoryProviders>
      <EmptiedQueue />
    </StoryProviders>
  ),
};

/** Dark, where `warning` has to stay a warning against the lifted ground. */
export const CaseNotHereDark: Story = {
  globals: { theme: "dark" },
  render: () => (
    <StoryProviders>
      <LinkedCaseNotice linked={ABSENT} />
    </StoryProviders>
  ),
};
