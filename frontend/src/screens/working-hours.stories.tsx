// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import {
  installFetchStub,
  jsonResponse,
  meRoute,
  StoryProviders,
} from "./story-utils";
import { WorkingHoursCard } from "./working-hours";

// When this reader is bookable, and on whose clock. Personal — an admin does
// not set a colleague's hours — so the states worth drawing are about what the
// reader themselves has decided: nothing yet, which says what customers are
// offered meanwhile, and a window they chose.

const meta: Meta<typeof WorkingHoursCard> = {
  title: "Settings/You/Account/Working hours",
  component: WorkingHoursCard,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof WorkingHoursCard>;

function Served({
  chosen,
  children,
}: Readonly<{ chosen: boolean; children: ReactNode }>) {
  installFetchStub({
    "GET /me": meRoute({}),
    "GET /me/working-hours": () =>
      jsonResponse({
        chosen,
        working_hours: chosen
          ? {
              start_time: "08:00",
              end_time: "16:30",
              days: [1, 2, 3, 4],
              timezone: "Europe/Berlin",
            }
          : {
              start_time: "09:00",
              end_time: "17:00",
              days: [1, 2, 3, 4, 5],
              timezone: "Europe/Berlin",
            },
      }),
  });
  return <StoryProviders>{children}</StoryProviders>;
}

/**
 * Nobody has chosen yet. The form is pre-filled with what customers are
 * offered meanwhile, and the notice says that this is the fallback rather than
 * somebody's decision — the whole difference between a setting and a guess.
 */
export const NothingChosenYet: Story = {
  render: () => (
    <Served chosen={false}>
      <WorkingHoursCard />
    </Served>
  ),
};

/** Hours this reader chose, on their own clock. */
export const Chosen: Story = {
  render: () => (
    <Served chosen>
      <WorkingHoursCard />
    </Served>
  ),
};
