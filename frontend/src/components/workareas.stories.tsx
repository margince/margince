// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { StoryProviders } from "../testing/story-utils";
import {
  ALL_AREAS,
  type PassportAreas,
  type WorkArea,
  WorkAreasCell,
  WorkAreasChoice,
} from "./workareas";

const ADMIN: WorkArea[] = [
  "business",
  "compliance",
  "capture",
  "workspace_settings",
  "admin_settings",
  "own_account",
];

function Choice({
  reachable,
  initial,
}: Readonly<{ reachable: readonly WorkArea[]; initial: PassportAreas }>) {
  const [value, setValue] = useState(initial);
  return (
    <WorkAreasChoice reachable={reachable} value={value} onChange={setValue} />
  );
}

const meta: Meta<typeof WorkAreasChoice> = {
  title: "Settings/You/AI access/Work areas",
  component: WorkAreasChoice,
};
export default meta;
type Story = StoryObj<typeof WorkAreasChoice>;

export const AllAreas: Story = {
  render: () => (
    <StoryProviders>
      <Choice reachable={ADMIN} initial={ALL_AREAS} />
    </StoryProviders>
  ),
};

export const ChosenAreas: Story = {
  render: () => (
    <StoryProviders>
      <Choice
        reachable={ADMIN}
        initial={{ mode: "custom", areas: ["business", "own_account"] }}
      />
    </StoryProviders>
  ),
};

// Unticking the last area leaves the grant incomplete, and the line says so.
export const NoneTicked: Story = {
  render: () => (
    <StoryProviders>
      <Choice reachable={ADMIN} initial={{ mode: "custom", areas: [] }} />
    </StoryProviders>
  ),
};

// A role that reaches one area draws nothing: the cell beside it stands in.
export const OneAreaRole: Story = {
  render: () => (
    <StoryProviders>
      <Choice reachable={["business"]} initial={ALL_AREAS} />
      <WorkAreasCell areas={{ mode: "custom", areas: ["business"] }} />
    </StoryProviders>
  ),
};

export const ListCell: Story = {
  render: () => (
    <StoryProviders>
      <WorkAreasCell areas={ALL_AREAS} />{" "}
      <WorkAreasCell
        areas={{ mode: "custom", areas: ["business", "admin_settings"] }}
      />
    </StoryProviders>
  ),
};
