// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { RollupsStrip } from "./projectreadings";
import { project360 } from "./projects.fixtures";
import { StoryProviders } from "./story-utils";

// The band under a project's header: what its deals are worth, what is still
// owed, and when anything last happened. A STRIP rather than five cards,
// because the five are read across as one comparison — which is also why the
// server withholds the whole plate rather than half of it, and why the states
// below are states of the PLATE.
//
// The fixture is `projects.fixtures.ts`, the one the project-page tests build
// from. A story that hand-rolled a second 360 would be a second answer to what
// a project looks like, and the two would drift.

const meta: Meta<typeof RollupsStrip> = {
  title: "Records/Project/Readings",
  component: RollupsStrip,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof RollupsStrip>;

function strip(view: ReturnType<typeof project360>) {
  return () => (
    <StoryProviders>
      <RollupsStrip view={view} />
    </StoryProviders>
  );
}

/** A project mid-delivery: every reading present, money and counts together. */
export const Delivering: Story = { render: strip(project360()) };

/**
 * The same plate at four digits, which is the only width where the notation is
 * visible at all: de-DE groups from a thousand, so a figure below it reads the
 * same in every locale and proves nothing about which one drew it.
 */
export const LargeFigures: Story = {
  render: strip(
    project360({
      rollups: {
        open_deal_value: { amount_minor: 428_150_000, currency: "EUR" },
        won_deal_value: { amount_minor: 91_400_000, currency: "EUR" },
        open_commitments: 1204,
        last_activity_at: "2026-07-01T09:00:00Z",
        activity_count: 48_213,
      },
    }),
  ),
};

/** The German plate: the same figures, the reader's own notation and the
 *  longer labels the layout has to hold. */
export const LargeFiguresGerman: Story = {
  render: () => (
    <StoryProviders locale="de">
      <RollupsStrip
        view={project360({
          rollups: {
            open_deal_value: { amount_minor: 428_150_000, currency: "EUR" },
            won_deal_value: { amount_minor: 91_400_000, currency: "EUR" },
            open_commitments: 1204,
            last_activity_at: "2026-07-01T09:00:00Z",
            activity_count: 48_213,
          },
        })}
      />
    </StoryProviders>
  ),
};

/**
 * A project nothing has been filed under yet. Zero is a READING here, not an
 * absence — "no work has landed on this project" is a true thing to report —
 * so the plate keeps its place and `last_activity_at` says never rather than
 * the slot going blank.
 */
export const NothingFiledYet: Story = {
  render: strip(
    project360({
      rollups: {
        open_deal_value: { amount_minor: 0, currency: "EUR" },
        won_deal_value: { amount_minor: 0, currency: "EUR" },
        open_commitments: 0,
        last_activity_at: null,
        activity_count: 0,
      },
    }),
  ),
};

/**
 * The plate withheld. A grant the reader lacks names `rollups` in
 * `sections_omitted`, and the strip says so instead of returning null —
 * a plate that vanished would read as a project with no money in it.
 */
export const Withheld: Story = {
  render: strip(
    project360({ sections_omitted: ["rollups"], rollups: undefined }),
  ),
};

/**
 * The read failed. Absent WITHOUT an omission is a different sentence from
 * absent because of a grant, and telling them apart is the whole reason
 * `stateOf` takes both the section and the payload.
 */
export const Unavailable: Story = {
  render: strip(project360({ rollups: undefined })),
};

/** At 390px the plate folds: the last slot takes the rest of its row, so a
 *  count that does not divide the columns leaves no orphan beside empty cells. */
export const Phone: Story = {
  tags: ["uat-phone"],
  render: strip(project360()),
};
