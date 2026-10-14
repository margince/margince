// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";
import { useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { Button } from "./atoms";
import { Disclosure, DisclosureGroup } from "./disclosure";

const meta = {
  title: "Components/Layout and structure/Disclosure",
  component: Disclosure,
  parameters: { layout: "padded" },
  args: { summary: "Matching rules", children: null },
} satisfies Meta<typeof Disclosure>;
export default meta;

type Story = StoryObj<typeof meta>;

const stack: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "1rem",
};

async function summaryNamed(canvasElement: HTMLElement, name: string) {
  const label = await within(canvasElement).findByText(name);
  const summary = label.closest("summary");
  if (summary === null) {
    throw new Error(`no summary reads ${name}`);
  }
  return summary;
}

// The fold is over once the box has let go of its measured height.
async function settled(summary: HTMLElement) {
  const box = summary.closest("details");
  await waitFor(() => expect(box?.hasAttribute("data-folding")).toBe(false));
}

export const ClosedAndOpen: Story = {
  render: () => (
    <div style={stack}>
      <Disclosure summary="Matching rules">
        <p className="t-caption">
          Closed by default for details the reader rarely needs.
        </p>
      </Disclosure>
      <Disclosure summary="Import log" open>
        <p className="t-caption">Open for a run or a new result.</p>
      </Disclosure>
    </div>
  ),
};

function ControlledExample() {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button onClick={() => setOpen(true)}>Open details</Button>
      <Disclosure summary="Editable details" open={open} onToggle={setOpen}>
        <p>The reader can close and reopen this section.</p>
      </Disclosure>
    </>
  );
}

export const Controlled: Story = {
  render: () => <ControlledExample />,
  play: async ({ canvasElement }) => {
    const summary = await summaryNamed(canvasElement, "Editable details");
    await userEvent.click(
      within(canvasElement).getByRole("button", { name: "Open details" }),
    );
    await expect(summary).toHaveAttribute("aria-expanded", "true");
    await userEvent.click(summary);
    await expect(summary).toHaveAttribute("aria-expanded", "false");
  },
};

const PARAGRAPHS = [
  "Every inbound lead is matched against the open deals of its company first, so a second contact at a customer joins the deal already in motion.",
  "A lead with no company match is scored against the ideal customer profile, and only a score above the threshold reaches a rep's worklist.",
  "Leads under the threshold wait in the nurture queue for thirty days, then leave it with a note on the record saying why.",
  "A rep who claims a lead by hand overrides every rule above, and the record keeps who claimed it and when.",
];

// Long enough that the fold takes its whole duration.
export const LongContent: Story = {
  render: () => (
    <Disclosure summary="How leads are routed">
      {PARAGRAPHS.map((text) => (
        <p key={text}>{text}</p>
      ))}
    </Disclosure>
  ),
  play: async ({ canvasElement }) => {
    const summary = await summaryNamed(canvasElement, "How leads are routed");
    await userEvent.click(summary);
    await expect(summary).toHaveAttribute("aria-expanded", "true");
    await settled(summary);
    await userEvent.click(summary);
    await expect(summary).toHaveAttribute("aria-expanded", "false");
    await settled(summary);
    await expect(summary.closest("details")).not.toHaveAttribute("open");
  },
};

export const MultiOpen: Story = {
  render: () => (
    <div>
      <Disclosure summary="Matching rules">
        <p>Rules run top to bottom.</p>
      </Disclosure>
      <Disclosure summary="Import log">
        <p>Last run: 214 rows, 3 skipped.</p>
      </Disclosure>
    </div>
  ),
  play: async ({ canvasElement }) => {
    const rules = await summaryNamed(canvasElement, "Matching rules");
    const log = await summaryNamed(canvasElement, "Import log");
    await userEvent.click(rules);
    await userEvent.click(log);
    await expect(rules).toHaveAttribute("aria-expanded", "true");
    await expect(log).toHaveAttribute("aria-expanded", "true");
  },
};

// A section drawn by a component of its own still joins the group around it.
function StagesSection() {
  return (
    <Disclosure summary="Stages">
      <p>Lead, Qualified, Proposal, Won, Lost.</p>
      <p>Won and Lost close a deal and always stay last.</p>
    </Disclosure>
  );
}

function GroupExample() {
  return (
    <DisclosureGroup>
      <Disclosure summary="Matching rules">
        {PARAGRAPHS.slice(0, 2).map((text) => (
          <p key={text}>{text}</p>
        ))}
      </Disclosure>
      <StagesSection />
      <Disclosure summary="Import log">
        <p>Last run: 214 rows, 3 skipped.</p>
      </Disclosure>
    </DisclosureGroup>
  );
}

export const Group: Story = {
  render: () => <GroupExample />,
  play: async ({ canvasElement }) => {
    const rules = await summaryNamed(canvasElement, "Matching rules");
    const stages = await summaryNamed(canvasElement, "Stages");
    await userEvent.click(rules);
    await expect(rules).toHaveAttribute("aria-expanded", "true");
    await settled(rules);
    await userEvent.click(stages);
    await expect(stages).toHaveAttribute("aria-expanded", "true");
    await expect(rules).toHaveAttribute("aria-expanded", "false");
    await settled(rules);
    await expect(rules.closest("details")).not.toHaveAttribute("open");
  },
};
export const GroupDark: Story = {
  ...Group,
  globals: { theme: "dark" },
};
export const GroupPhone: Story = {
  ...Group,
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

export const WithAction: Story = {
  render: () => (
    <Disclosure summary="Thread" action={<Button>Full history</Button>}>
      <p>Three messages since Monday.</p>
    </Disclosure>
  ),
  play: async ({ canvasElement }) => {
    const summary = await summaryNamed(canvasElement, "Thread");
    await userEvent.click(
      within(canvasElement).getByRole("button", { name: "Full history" }),
    );
    await expect(summary).toHaveAttribute("aria-expanded", "false");
    await userEvent.click(summary);
    await expect(summary).toHaveAttribute("aria-expanded", "true");
  },
};
