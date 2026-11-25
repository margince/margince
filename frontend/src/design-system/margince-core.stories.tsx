// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { Button } from "./atoms";
import { MarginceCoreScene } from "./margince-core";
import { BEHAVIOUR } from "./margince-core-motion";

/**
 * WDS-CORE-2 says the state vocabulary is closed. A catalog showing three of
 * the five is how a closed vocabulary quietly becomes an open one, so every
 * state gets a story, including the two nobody demos: `warning` and `error`,
 * the states a reviewer never asks to see and a user meets on a bad day.
 *
 * **The vocabulary is the agent's work lifecycle**, in order: idle → ingest →
 * working, and back to idle when a run settles, plus the two ways it stops.
 * There is no `listening`: Margince's agent works overnight over captured
 * activity and stages proposals a human confirms — it never holds a
 * conversation, and a state naming one would be the product claiming
 * something it does not do.
 *
 * **State is motion first and colour second.** Each state owns one distinct
 * `level` / `form` / `speed` / `pulse` / `ingest` signature
 * (`margince-core-motion.ts`), driven straight into the shader, before it owns a
 * colour triple (`margince-core.css`). Two consequences worth knowing before
 * reading the five:
 *
 *  - A still frame is not the story. `ingest` and `working` share the thinking
 *    figure and are told apart by how the ribbons move: one falls inward, the
 *    other turns and breathes. Idle drifts slowly at a lower frame rate;
 *    warning and error hold still, so the loop parks.
 *    `margince-core-motion.test.ts` pins the signature.
 *  - The condition a user acts on is never the orb. Every surface that shows a
 *    Core also states its condition in words beside it, which is what makes the
 *    orb safe to be `aria-hidden`.
 *
 * `Ladder` at the end of this file shows all five at once, which is the view that
 * catches two states having drifted into looking alike.
 */
const meta = {
  title: "Components/AI and provenance/Margince core",
  component: MarginceCoreScene,
  parameters: { layout: "centered" },
} satisfies Meta<typeof MarginceCoreScene>;
export default meta;

type Story = StoryObj<typeof meta>;

/**
 * Rest, where the Core spends nearly all of its life: nothing staged, nothing
 * running. The calm figure drifts slowly, since more motion would read as work.
 * A finished run settles back here.
 */
export const Idle: Story = { args: { state: "idle" } };

/**
 * Captured calls, mail and meetings arriving. `ingest` is at its ceiling here
 * alone: grain streams from the ribbons to the centre, so material comes in.
 */
export const Ingesting: Story = { args: { state: "ingest" } };

/**
 * Traversing the context graph, matching records against evidence, composing
 * staged proposals. The thinking figure: brighter, twisted, folded, breathing,
 * and the fastest phase of the five.
 */
export const Working: Story = { args: { state: "working" } };

/**
 * The ring is the optional half of WDS-CORE-2, and a ring rather than a bar
 * because the Core is already the thing being waited on.
 *
 * `progress` is genuinely optional rather than defaulted to 0: omit it and no
 * ring renders at all, which is why every other story here has none. A 0% ring
 * and no ring say different things — one is a job that has not moved, the other
 * is a job with no measurable length.
 */
export const IngestWithProgress: Story = {
  args: { state: "ingest", progress: 0.58 },
};

/**
 * Under prefers-reduced-motion the ring and the fallback dress change at once.
 * With the setting on (in the system, or the browser's rendering emulation),
 * stepping the ring moves it in one frame with no sweep. With it off, each step
 * sweeps over 0.4s.
 */
export const ProgressSteps: Story = {
  args: { state: "ingest" },
  render: (args) => {
    const [progress, setProgress] = useState(0.2);
    return (
      <div
        style={{
          display: "grid",
          gap: "var(--space-4)",
          justifyItems: "center",
        }}
      >
        <MarginceCoreScene {...args} progress={progress} />
        <Button
          onClick={() => setProgress((now) => (now >= 0.8 ? 0.2 : now + 0.3))}
        >
          Step the ring
        </Button>
      </div>
    );
  },
};

/**
 * Stopped and waiting for a contact: a record contradicts reality, or an action
 * lacks a permission, a source or a licence. The thinking figure at rest, in
 * amber. The orb says only that a contact is needed; the surface says which.
 */
export const Warning: Story = { args: { state: "warning" } };

/**
 * The run failed. The thinking figure at rest, in red, the one colour outside
 * the product's palette.
 */
export const Errored: Story = { args: { state: "error" } };

/**
 * Both size presets at once, because the difference is not only 230px against
 * 150px.
 *
 * The point count follows the pixel area, so a small ball carries fewer points,
 * not smaller ones. Review it at a desktop width: below 900px the hero takes
 * the md geometry.
 */
export const Sizes: Story = {
  render: () => (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: "var(--space-6)",
      }}
    >
      <MarginceCoreScene state="working" />
      <MarginceCoreScene state="working" size="md" />
    </div>
  ),
};

/**
 * The two ways a Core is dressed. On a dark surface it is emissive: it adds its
 * own light and the ribbons are the object. On paper it goes opaque and dark,
 * with the ribbons glowing inside it — an emissive ball on white is a smudge, so
 * `surface="dark"` exists for hosts that are dark in both themes, like the
 * workspace rail, and never follows the page's own theme the way `auto` does.
 */
export const Surfaces: Story = {
  render: () => (
    <div
      style={{
        display: "flex",
        gap: "var(--space-6)",
      }}
    >
      <div style={{ padding: "var(--space-6)", background: "var(--bgPage)" }}>
        <MarginceCoreScene state="working" size="md" surface="auto" />
      </div>
      <div
        style={{ padding: "var(--space-6)", background: "var(--bgInkField)" }}
      >
        <MarginceCoreScene state="working" size="md" surface="dark" />
      </div>
    </div>
  ),
};

/**
 * Every state at once, in lifecycle order.
 *
 * This is the review a per-state story cannot give: two states drifting into the
 * same movement is invisible when they sit on separate pages and obvious here.
 * Read across the rows — each state should be nameable without its caption.
 */
export const Ladder: Story = {
  render: () => (
    <div
      style={{
        display: "grid",
        gridTemplateColumns: "repeat(4, minmax(0, 1fr))",
        gap: "var(--space-5)",
      }}
    >
      {Object.keys(BEHAVIOUR).map((state) => (
        <figure
          key={state}
          style={{
            margin: 0,
            display: "grid",
            justifyItems: "center",
            gap: "var(--space-2)",
          }}
        >
          <MarginceCoreScene
            state={state as keyof typeof BEHAVIOUR}
            size="md"
            feed={false}
          />
          <figcaption>{state}</figcaption>
        </figure>
      ))}
    </div>
  ),
};

/**
 * A state change eases. Press to go from idle to working: the figure twists,
 * folds and brightens over about a second, then slows again back in idle.
 */
export const StateChange: Story = {
  render: () => {
    const [working, setWorking] = useState(false);
    return (
      <div
        style={{
          display: "grid",
          gap: "var(--space-4)",
          justifyItems: "center",
        }}
      >
        <MarginceCoreScene state={working ? "working" : "idle"} size="md" />
        <Button onClick={() => setWorking((now) => !now)}>
          {working ? "Back to idle" : "Start working"}
        </Button>
      </div>
    );
  },
};

/**
 * The feed off, which is the honest setting wherever nothing is arriving: `feed`
 * raises the intake floor the shader draws with, so a Core that is merely
 * present must not carry it.
 *
 * It is also what a Core sitting next to copy needs. The agent rail
 * (agentrail.tsx) and the voice build scene (voice-scenes.build.tsx) both run
 * `feed={false}` for exactly that reason.
 */
export const WithoutFeed: Story = {
  args: { state: "idle", feed: false },
};
