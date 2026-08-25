// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { ReactNode } from "react";
import { useState } from "react";
import { userEvent, within } from "storybook/test";
import { Badge, Button } from "../design-system/atoms";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { StoryProviders } from "../screens/story-utils";
import {
  AttentionProvider,
  useAttention,
  usePublishScope,
  usePublishSelection,
} from "./attention";

// The channel a screen states its own scope on: what it is about beyond its
// route, so the chrome can report it without reaching into the screen to guess.
//
// Nothing here draws anything, which is exactly why it needs a story: the only
// way to look at a channel is to put a publisher and a reader on either end of
// it and show what arrives. The harnesses below stand in for a list screen and
// for the agent bar — the two real ends — and what they render is the SCOPE,
// which is the thing under review.
//
// Two properties are worth seeing rather than reading about. The direction:
// screens publish, chrome reads, and no arrow points the other way. And the
// wording: `usePublishSelection` owns the sentence, so two lists cannot word
// one fact differently — which is also why the count is formatted, and why a
// four-digit selection is one of the stories.

/** Stands in for the agent bar: the one thing that READS the channel. */
function Chrome() {
  const scope = useAttention();
  return (
    <PanelRow>
      <span className="t-caption">chrome reads</span>{" "}
      {scope ? <Badge tone="accent">{scope.label}</Badge> : <span>—</span>}
    </PanelRow>
  );
}

/** Stands in for a list screen: it selects rows and says so, nothing more. */
function SelectingList({ rows }: Readonly<{ rows: number }>) {
  const [selected, setSelected] = useState(0);
  usePublishSelection(selected);
  return (
    <PanelBody>
      <p className="t-caption">
        A list of {rows} rows. It publishes its selection and reads nothing.
      </p>
      <div className="card-actions">
        <Button variant="primary" small onClick={() => setSelected(rows)}>
          Select all
        </Button>
        <Button small onClick={() => setSelected(0)}>
          Clear
        </Button>
      </div>
    </PanelBody>
  );
}

/** A screen whose scope is not a selection at all. */
function FilteredList({ label }: Readonly<{ label: string | null }>) {
  usePublishScope(label);
  return (
    <PanelBody>
      <p className="t-caption">
        A screen stating a scope that is not a count: {label ?? "nothing"}.
      </p>
    </PanelBody>
  );
}

function wired(children: ReactNode) {
  return () => (
    <StoryProviders>
      <AttentionProvider>
        <Panel title="One screen, one chrome, one channel">
          {children}
          <Chrome />
        </Panel>
      </AttentionProvider>
    </StoryProviders>
  );
}

// Pressing the list's own verb, so the published sentence is one the list
// really sent rather than one the story wrote.
const selectAll: NonNullable<Story["play"]> = async ({ canvasElement }) => {
  const verb = await within(canvasElement).findByRole("button", {
    name: "Select all",
  });
  await userEvent.setup().click(verb);
};

const meta: Meta<typeof AttentionProvider> = {
  title: "Shell/Attention channel",
  component: AttentionProvider,
  parameters: { layout: "padded" },
};
export default meta;
type Story = StoryObj<typeof AttentionProvider>;

/**
 * Nothing selected. The channel carries null rather than "0 selected": a scope
 * is what is TRUE about the screen, and "none of them" is not something the bar
 * has anything to say about.
 */
export const NothingPublished: Story = {
  render: wired(<SelectingList rows={8} />),
};

/** Rows selected, and the sentence arrived. The `play` presses the list's own
 *  verb rather than the story seeding a count, so what is on screen is what the
 *  channel actually carried. */
export const SelectionPublished: Story = {
  render: wired(<SelectingList rows={8} />),
  play: selectAll,
};

/**
 * A selection wide enough to be written in a notation. Four digits is where
 * de-DE first groups, so it is the only width at which the sentence proves
 * whose notation drew it — below it the figure reads the same everywhere and
 * the formatting is untestable by eye.
 */
export const LargeSelection: Story = {
  render: wired(<SelectingList rows={1204} />),
  play: selectAll,
};

/** The same channel in German. The count and the sentence around it come from
 *  one place, so they cannot disagree about which language they are in. */
export const LargeSelectionGerman: Story = {
  render: () => (
    <StoryProviders locale="de">
      <AttentionProvider>
        <Panel title="Ein Bildschirm, eine Leiste, ein Kanal">
          <SelectingList rows={1204} />
          <Chrome />
        </Panel>
      </AttentionProvider>
    </StoryProviders>
  ),
};

/** A scope that is not a count. `usePublishScope` is the general case, and a
 *  screen with nothing countable still has something to say. */
export const NonCountScope: Story = {
  render: wired(<FilteredList label="Owned by me · this quarter" />),
};

/**
 * No provider above the publisher at all — a screen rendered on its own, in a
 * story or before the shell mounts. Publishing into nothing is correct here
 * rather than a fault, so the list draws and the channel simply has no reader.
 */
export const NoChromeListening: Story = {
  render: () => (
    <StoryProviders>
      <Panel title="A screen with no chrome above it">
        <SelectingList rows={8} />
      </Panel>
    </StoryProviders>
  ),
};
