// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect, useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { Button } from "./atoms";
import { DataTable } from "./datatable";
import { Panel } from "./panel";

// A generic component takes no `component` here: Storybook would have to infer
// `Row` from nothing to derive the args table.
const meta: Meta = {
  title: "Components/Text and data display/Data table row detail",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

type DemoRun = { id: string; rule: string; when: string; result: string };
const RUNS: DemoRun[] = [
  {
    id: "r1",
    rule: "Welcome new leads",
    when: "Today, 09:12",
    result: "Sent 3 emails",
  },
  {
    id: "r2",
    rule: "Nudge a quiet deal",
    when: "Yesterday, 17:40",
    result: "Skipped: deal was won",
  },
  {
    id: "r3",
    rule: "Tag inbound replies",
    when: "Yesterday, 08:05",
    result: "Tagged 12 messages",
  },
];

function RunsPanel({ single }: Readonly<{ single?: boolean }>) {
  return (
    <Panel title="Rule runs">
      <DataTable<DemoRun>
        bleed
        fold
        label="Rule runs"
        rows={RUNS}
        rowKey={(run) => run.id}
        columns={[
          { key: "rule", header: "Rule", render: (run) => run.rule },
          { key: "when", header: "When", render: (run) => run.when },
        ]}
        detail={{
          header: "Detail",
          toggleLabel: (run) => `Show the run of ${run.rule}`,
          render: (run) => <p>{run.result}</p>,
          single,
        }}
      />
    </Panel>
  );
}

export const RowDetail: Story = {
  render: () => <RunsPanel />,
  play: async ({ canvasElement }) => {
    const toggle = await within(canvasElement).findByRole("button", {
      name: "Show the run of Welcome new leads",
    });
    await userEvent.click(toggle);
    await expect(toggle.getAttribute("aria-expanded")).toBe("true");
  },
};
// `single`: opening a row closes the open one, both folds moving at once.
export const RowDetailSingle: Story = {
  render: () => <RunsPanel single />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const first = await canvas.findByRole("button", {
      name: "Show the run of Welcome new leads",
    });
    const second = canvas.getByRole("button", {
      name: "Show the run of Nudge a quiet deal",
    });
    await userEvent.click(first);
    await expect(first).toHaveAttribute("aria-expanded", "true");
    await userEvent.click(second);
    await expect(second).toHaveAttribute("aria-expanded", "true");
    await expect(first).toHaveAttribute("aria-expanded", "false");
    await waitFor(() =>
      expect(canvas.queryByText("Sent 3 emails")).not.toBeInTheDocument(),
    );
  },
};
// A detail whose read lands late: while the row opens (the first row), or once
// it has opened (the second). The fold follows the content and never jumps.
const LANDS_AFTER: Readonly<Record<string, number>> = { r1: 60, r2: 400 };

function LateCriteria({ run }: Readonly<{ run: DemoRun }>) {
  const [loaded, setLoaded] = useState(false);
  useEffect(() => {
    const landing = setTimeout(() => setLoaded(true), LANDS_AFTER[run.id] ?? 0);
    return () => clearTimeout(landing);
  }, [run.id]);
  if (!loaded) {
    return <p className="t-caption">Loading the run…</p>;
  }
  return (
    <div style={{ display: "grid", gap: "var(--space-2)" }}>
      <p className="t-caption">What {run.rule} did</p>
      <p>{run.result}</p>
      <div>
        <Button>Run again</Button>
      </div>
    </div>
  );
}

type FoldFrame = Readonly<{ height: number; folding: boolean }>;

// Each painted frame of one row's fold. Read after the frame, so a height the
// fold restores before paint is not counted as drawn.
function sampleFold(cell: HTMLElement) {
  const frames: FoldFrame[] = [];
  let frame = 0;
  let after: ReturnType<typeof setTimeout> | undefined;
  const sample = () => {
    after = setTimeout(() => {
      const fold = cell.querySelector<HTMLElement>(".datatable-detail-fold");
      if (fold !== null && !cell.closest("tr")?.hidden) {
        frames.push({
          height: fold.getBoundingClientRect().height,
          folding: fold.hasAttribute("data-folding"),
        });
      }
      frame = requestAnimationFrame(sample);
    }, 0);
  };
  frame = requestAnimationFrame(sample);
  const stop = () => {
    cancelAnimationFrame(frame);
    clearTimeout(after);
  };
  return { frames, stop };
}

// A height that changes between two frames with no fold moving is a jump.
function jumps(frames: readonly FoldFrame[]): string[] {
  return frames.slice(1).flatMap((frame, index) => {
    const before = frames[index] ?? frame;
    const moved = Math.abs(frame.height - before.height) >= 1;
    return moved && !frame.folding && !before.folding
      ? [`${before.height} to ${frame.height}`]
      : [];
  });
}

export const RowDetailLateContent: Story = {
  render: () => (
    <Panel title="Rule runs">
      <DataTable<DemoRun>
        bleed
        label="Rule runs"
        rows={RUNS}
        rowKey={(run) => run.id}
        columns={[{ key: "rule", header: "Rule", render: (run) => run.rule }]}
        detail={{
          header: "Detail",
          toggleLabel: (run) => `Show the run of ${run.rule}`,
          render: (run) => <LateCriteria run={run} />,
        }}
      />
    </Panel>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    for (const rule of ["Welcome new leads", "Nudge a quiet deal"]) {
      const toggle = await canvas.findByRole("button", {
        name: `Show the run of ${rule}`,
      });
      const cell = canvasElement.ownerDocument.getElementById(
        toggle.getAttribute("aria-controls") ?? "",
      );
      if (cell === null) throw new Error(`${rule} controls no detail`);
      const { frames, stop } = sampleFold(cell);
      try {
        await userEvent.click(toggle);
        await canvas.findByText(`What ${rule} did`, {}, { timeout: 2000 });
        const landed = frames.length;
        await waitFor(() => {
          expect(frames.length).toBeGreaterThan(landed + 2);
          expect(frames.at(-1)?.folding).toBe(false);
        });
      } finally {
        stop();
      }
      await expect(jumps(frames)).toEqual([]);
      await expect(frames.slice(1).some((frame) => frame.folding)).toBe(true);
    }
  },
};
export const RowDetailDark: Story = {
  ...RowDetail,
  globals: { theme: "dark" },
};
export const RowDetailPhone: Story = {
  ...RowDetail,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};
