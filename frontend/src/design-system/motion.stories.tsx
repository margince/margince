// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { RefreshCw } from "lucide-react";
import { type CSSProperties, useState } from "react";
import { Button } from "./atoms";
import { usePrefersReducedMotion } from "./motion";
import {
  specimenColumn,
  specimenIntro,
  specimenNote,
  specimenToken,
  useTokenValue,
} from "./tokenspecimen";

/**
 * Three durations and three curves, named by the job they time and not by
 * their length, so two controls side by side agree. Motion is spent on arrival
 * and on state, never as decoration, and only `transform` and `opacity`
 * animate.
 *
 * Under `prefers-reduced-motion` every motion here jumps to its end state, and
 * so does the product. These specimens do the same, so on a machine that asks
 * for less motion they will not move; the preference is read live.
 */
const meta = {
  title: "Foundations/Motion",
  parameters: { layout: "padded" },
} satisfies Meta;
export default meta;

type Story = StoryObj<typeof meta>;

const DOT = "var(--space-4)";
const TRACK = "calc(var(--space-16) * 4)";

const grid: CSSProperties = {
  display: "grid",
  gridTemplateColumns: "minmax(200px, 1fr) minmax(0, 3fr) minmax(0, 1fr)",
  gap: "var(--space-4)",
  alignItems: "center",
};
const track: CSSProperties = {
  inlineSize: `calc(${TRACK} + var(--space-4))`,
  padding: "var(--space-2)",
  background: "var(--bgElevated)",
  border: "1px solid var(--borderSubtle)",
  borderRadius: "var(--r-full)",
};
const dot: CSSProperties = {
  inlineSize: DOT,
  blockSize: DOT,
  borderRadius: "var(--r-full)",
  background: "var(--accent)",
};

const SWAP_HINT = "Each press sends every dot to the other end.";

type Move = Readonly<{ token: string; usage: string }>;

function Replay({ onReplay, hint }: { onReplay: () => void; hint: string }) {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: "var(--gapActions)",
      }}
    >
      <Button variant="ghost" onClick={onReplay}>
        <RefreshCw aria-hidden />
        Replay
      </Button>
      <span className={specimenNote}>{hint}</span>
    </div>
  );
}

function Runner({
  token,
  usage,
  far,
  duration,
  easing,
}: Move & { far: boolean; duration: string; easing: string }) {
  const { ref, value } = useTokenValue<HTMLDivElement>(token);
  const reduced = usePrefersReducedMotion();
  return (
    <div style={grid}>
      <div className={specimenColumn}>
        <code className={specimenToken}>{token}</code>
        <span className={specimenNote}>{usage}</span>
      </div>
      <div style={track}>
        <div
          ref={ref}
          style={{
            ...dot,
            transform: far ? `translateX(${TRACK})` : "none",
            transition: reduced ? "none" : `transform ${duration} ${easing}`,
          }}
        />
      </div>
      <code>{value}</code>
    </div>
  );
}

const DURATIONS: readonly Move[] = [
  {
    token: "--dur-tap",
    usage: "A press answering: the floor of perceptible response.",
  },
  {
    token: "--dur-state",
    usage: "A control changing in place: hover, focus, a crossfade.",
  },
  {
    token: "--dur-move",
    usage: "Something arriving or travelling: a panel, the rail, a knob.",
  },
  {
    token: "--dur-enter",
    usage:
      "Longer than a move, and it times a fade only: content arriving on a page or in a tab panel.",
  },
];

/**
 * Four durations, all travelling on the same curve so the length is the only
 * thing that differs. The rise of an arrival finishes at `--dur-move` while its
 * fade keeps going to `--dur-enter`, because one duration for both gives either
 * a sluggish rise or a hard fade.
 */
export const Durations: Story = {
  render: () => {
    const [far, setFar] = useState(false);
    return (
      <div className={specimenColumn} style={{ gap: "var(--space-4)" }}>
        <p className={specimenIntro}>
          Four durations, named by the job they time and not by their length, so
          two controls side by side agree. Use --dur-tap for a press answering,
          --dur-state for a control changing in place, --dur-move for something
          arriving or travelling, and --dur-enter for a fade only. Only
          transform and opacity animate. Under prefers-reduced-motion everything
          jumps to its end state, and these specimens stay still too.
        </p>
        <Replay hint={SWAP_HINT} onReplay={() => setFar((now) => !now)} />
        {DURATIONS.map((move) => (
          <Runner
            key={move.token}
            {...move}
            far={far}
            duration={`var(${move.token})`}
            easing="var(--ease-out)"
          />
        ))}
      </div>
    );
  },
};

const CURVES: readonly Move[] = [
  {
    token: "--ease-out",
    usage:
      "Deceleration, for anything that arrives and settles: it leaves fast and lands slow.",
  },
  {
    token: "--ease-in-out",
    usage:
      "Symmetric, for a change that is not an arrival: a colour, a ground, a knob sliding between two places.",
  },
  {
    token: "--ease-spring",
    usage:
      "The one curve that overshoots, for a press releasing and nothing else. On anything larger than a control it reads as the UI being pleased with itself.",
  },
];

/**
 * Three curves over the longest duration, so the shape of each is visible. The
 * spring overshoots the end of the track before it settles.
 */
export const Easing: Story = {
  render: () => {
    const [far, setFar] = useState(false);
    return (
      <div className={specimenColumn} style={{ gap: "var(--space-4)" }}>
        <p className={specimenIntro}>
          Three curves. Use --ease-out for anything that arrives and settles,
          --ease-in-out for a change that is not an arrival, and --ease-spring
          only for a press releasing. Under prefers-reduced-motion the dots jump
          to the end.
        </p>
        <Replay hint={SWAP_HINT} onReplay={() => setFar((now) => !now)} />
        {CURVES.map((move) => (
          <Runner
            key={move.token}
            {...move}
            far={far}
            duration="var(--dur-enter)"
            easing={`var(${move.token})`}
          />
        ))}
      </div>
    );
  },
};

const BLOCKS = ["Heading", "Summary", "Card", "List", "Footer", "Notes"];

const block: CSSProperties = {
  padding: "var(--space-3)",
  background: "var(--bgElevated)",
  border: "1px solid var(--borderSubtle)",
  borderRadius: "var(--r-sm)",
};

/**
 * `--stagger-enter` is the gap between one arriving block and the next. It is
 * small on purpose: a reading order made briefly visible, not a sequence
 * anyone waits through, so six blocks are all moving within a fifth of a
 * second. The blocks use the product's own `.arrive-stack`, so this is the
 * real arrival and not a copy of it.
 */
export const Stagger: Story = {
  render: () => {
    const [run, setRun] = useState(0);
    const { ref, value } = useTokenValue<HTMLDivElement>("--stagger-enter");
    return (
      <div ref={ref} className={specimenColumn} style={{ maxWidth: "480px" }}>
        <p className={specimenIntro}>
          Blocks arriving on a page enter in reading order, one --stagger-enter
          apart, so a stack reads as being dealt and not as a flash. Use
          .arrive-stack on the container and let its children arrive. Under
          prefers-reduced-motion the blocks appear at once.
        </p>
        <Replay
          hint="Each press plays the arrival again."
          onReplay={() => setRun((count) => count + 1)}
        />
        <code className={specimenToken}>--stagger-enter {value}</code>
        <div key={run} className={`arrive-stack ${specimenColumn}`}>
          {BLOCKS.map((name) => (
            <div key={name} style={block}>
              {name}
            </div>
          ))}
        </div>
      </div>
    );
  },
};
