// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { Button, Card, SectionHeader } from "./atoms";
import { Row, Stack } from "./stack";

const meta: Meta<typeof Stack> = {
  title: "Design System/Stack",
  component: Stack,
};
export default meta;

/**
 * The shape an extension unit assembles: a heading, some rows, and a verb —
 * spaced by the scale rather than by whatever the browser does with two
 * adjacent divs.
 */
export const UnitCard: StoryObj = {
  render: () => (
    <Card>
      <Stack gap="4">
        <SectionHeader title="Who is captured" />
        <Stack gap="2">
          <Row justify="between">
            <span>Thuy An</span>
            <Button variant="ghost">Take off</Button>
          </Row>
          <Row justify="between">
            <span>Marek Sobota</span>
            <Button variant="ghost">Take off</Button>
          </Row>
        </Stack>
        <Row justify="end">
          <Button variant="primary">Save</Button>
        </Row>
      </Stack>
    </Card>
  ),
};

/** Every step on the scale, so a reader can see what they are choosing between. */
export const Steps: StoryObj = {
  render: () => (
    <Stack gap="6">
      {(["1", "2", "3", "4", "6"] as const).map((step) => (
        <Stack key={step} gap="1">
          <span className="t-caption">gap={step}</span>
          <Row gap={step}>
            <Button variant="ghost">one</Button>
            <Button variant="ghost">two</Button>
            <Button variant="ghost">three</Button>
          </Row>
        </Stack>
      ))}
    </Stack>
  ),
};
