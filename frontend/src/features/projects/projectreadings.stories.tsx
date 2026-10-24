// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { project360 } from "../../testing/projects.fixtures";
import { StoryProviders } from "../../testing/story-utils";
import { ProjectFigures } from "./projectreadings";
import { deliveringProject360 } from "./projectstanding.fixtures";

// The figures at the foot of a project's How it stands: phase time, open
// commitments and the files. Each empty or withheld cell says so in words,
// never a bare zero.

const meta: Meta<typeof ProjectFigures> = {
  title: "Records/Project 360/Figures",
  component: ProjectFigures,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof ProjectFigures>;

function strip(view: ReturnType<typeof project360>, locale?: "de") {
  return () => (
    <StoryProviders locale={locale}>
      <ProjectFigures view={view} onOpenDocuments={() => {}} />
    </StoryProviders>
  );
}

/** Mid-delivery: 134 days in Delivering, three open tasks with one overdue,
 *  one file attached. */
export const Delivering: Story = { render: strip(deliveringProject360()) };

/** The same figures in German, with the longer labels the cells must hold. */
export const DeliveringGerman: Story = {
  render: strip(deliveringProject360(), "de"),
};

/** A new project: no task open, no file attached. */
export const NothingYet: Story = {
  render: strip(
    project360({
      rollups: {
        open_deal_value: { amount_minor: null, currency: null },
        won_deal_value: { amount_minor: null, currency: null },
        open_commitments: 0,
        last_activity_at: null,
        activity_count: 0,
      },
    }),
  ),
};

/** The commitments withheld from this reader's role. */
export const Withheld: Story = {
  render: strip(
    project360({ sections_omitted: ["commitments"], commitments: undefined }),
  ),
};

/** At 390px the plate folds to one row per figure. */
export const Phone: Story = {
  tags: ["uat-phone"],
  render: strip(deliveringProject360()),
};

export const DeliveringDark: Story = {
  globals: { theme: "dark" },
  render: strip(deliveringProject360()),
};

export const UnpricedDeals: Story = {
  render: strip(
    project360({
      rollups: {
        open_deal_value: { amount_minor: null, currency: "EUR" },
        won_deal_value: { amount_minor: 450_000, currency: "EUR" },
        open_deals_unpriced: 2,
        won_deals_unpriced: 0,
        open_commitments: 0,
        last_activity_at: null,
        activity_count: 0,
      },
    }),
  ),
};
