// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { StoryProviders } from "../screens/story-utils";
import { AppErrorBoundary } from "./errorboundary";

// What a reader meets when a screen throws mid-render. Without the boundary the
// first throw unmounts the whole document and leaves a white page nobody can
// act on; with it the reader loses the view and keeps everything else.
//
// The two frames are the failure and its absence, because the boundary's own
// claim is that the SECOND one is what the rest of the application still looks
// like. The fallback shows nothing of the error itself — a render throw names
// our internals and the reader cannot act on it — so what is worth a picture is
// the sentence and the way back.

function Fragile(): never {
  throw new Error("cannot read properties of undefined (reading 'rows')");
}

function RoutedShell() {
  return (
    <div className="wrap narrow">
      <p className="t-body">The routed shell, still rendering.</p>
    </div>
  );
}

const meta: Meta<typeof AppErrorBoundary> = {
  title: "Shell/Render failure",
  component: AppErrorBoundary,
};
export default meta;

type Story = StoryObj<typeof AppErrorBoundary>;

/**
 * A screen that threw. React reports every error a boundary catches through
 * console.error, so a story whose SUBJECT is a caught throw trips the render
 * gate's console rule by doing exactly what it exists to show. The tag is that
 * gate's declared opt-out (frontend/scripts/fe-uat.mjs), scoped to this story.
 */
export const ViewStoppedWorking: Story = {
  tags: ["uat-expected-console-error"],
  render: () => (
    <StoryProviders>
      <AppErrorBoundary>
        <Fragile />
      </AppErrorBoundary>
    </StoryProviders>
  ),
};

/** Nothing thrown: the boundary draws nothing and the screen renders as itself. */
export const NothingThrown: Story = {
  render: () => (
    <StoryProviders>
      <AppErrorBoundary>
        <RoutedShell />
      </AppErrorBoundary>
    </StoryProviders>
  ),
};

/** The same failure in dark, where the retry has to lift off the empty state. */
export const ViewStoppedWorkingDark: Story = {
  tags: ["uat-expected-console-error"],
  globals: { theme: "dark" },
  render: () => (
    <StoryProviders>
      <AppErrorBoundary>
        <Fragile />
      </AppErrorBoundary>
    </StoryProviders>
  ),
};
