// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { CSSProperties } from "react";
import { useState } from "react";
import { expect, waitFor } from "storybook/test";
import { useT } from "../i18n";
import { StoryProviders } from "../testing/story-utils";
import { Card, Kbd, SegmentedControl } from "./atoms";

const meta: Meta<typeof SegmentedControl> = {
  title: "Components/Forms and input/Segmented control",
  component: SegmentedControl,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof SegmentedControl>;

const row: CSSProperties = {
  display: "flex",
  gap: "0.75rem",
  alignItems: "center",
  flexWrap: "wrap",
};
const stack: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: "1rem",
};

const RANGES = ["month", "quarter", "year"] as const;
type Range = (typeof RANGES)[number];
const RANGE_LABELS: Record<Range, string> = {
  month: "Month",
  quarter: "Quarter",
  year: "Year",
};

const SIDES = ["owner", "team"] as const;
type Side = (typeof SIDES)[number];
const SIDE_LABELS: Record<Side, string> = { owner: "Owner", team: "Team" };

const TABS = ["overview", "contacts", "deals"] as const;
type Tab = (typeof TABS)[number];
const TAB_LABELS: Record<Tab, string> = {
  overview: "360",
  contacts: "Contacts",
  deals: "Deals",
};

// Controlled: without state here the buttons never move.
function ToolbarDemo() {
  const [range, setRange] = useState<Range>("quarter");
  const [side, setSide] = useState<Side>("owner");
  const [tab, setTab] = useState<Tab>("overview");
  return (
    <div style={stack}>
      <div style={{ ...row, justifyContent: "space-between" }}>
        <SegmentedControl
          options={RANGES}
          value={range}
          onChange={setRange}
          labels={RANGE_LABELS}
          label="Reporting range"
        />
        <span className="t-caption">
          Press <Kbd>/</Kbd> to search, <Kbd>Ctrl</Kbd> <Kbd>K</Kbd> for the
          command bar, <Kbd>Esc</Kbd> to close.
        </span>
      </div>
      <SegmentedControl
        options={SIDES}
        value={side}
        onChange={setSide}
        labels={SIDE_LABELS}
        label="Target amount"
      />
      {/* Partial counts: an option with none is not the explicit zero beside
          it. */}
      <SegmentedControl
        options={TABS}
        value={tab}
        onChange={setTab}
        labels={TAB_LABELS}
        counts={{ contacts: 6, deals: 0 }}
        label="Record section"
      />
    </div>
  );
}

// Two options and three: a two-up control has no middle segment, which is
// where divider rules written for three break.
export const Toolbar: Story = {
  render: () => <ToolbarDemo />,
};

const RECORD_TABS = ["overview", "research", "documents"] as const;
type RecordTab = (typeof RECORD_TABS)[number];
const RECORD_TAB_LABELS: Record<RecordTab, string> = {
  overview: "Overview",
  research: "Data and tools",
  documents: "Documents",
};

function MarkedTabsDemo() {
  const [tab, setTab] = useState<RecordTab>("overview");
  return (
    <SegmentedControl
      options={RECORD_TABS}
      value={tab}
      onChange={setTab}
      labels={RECORD_TAB_LABELS}
      label="Record sections"
      marks={{ research: true }}
    />
  );
}

// The dot is `aria-hidden` and never the only carrier: the surface it points
// at states the fact in words.
export const MarkedOption: Story = {
  render: () => <MarkedTabsDemo />,
};

const HOURS = ["8", "9", "13", "17"] as const;
type Hour = (typeof HOURS)[number];
const HOUR_LABELS: Record<Hour, string> = {
  "8": "08:00",
  "9": "09:00",
  "13": "13:00",
  "17": "17:00",
};

function RefusedDemo() {
  const [hour, setHour] = useState<Hour>("17");
  return (
    <div style={stack}>
      <SegmentedControl
        options={HOURS}
        value={hour}
        onChange={setHour}
        labels={HOUR_LABELS}
        label="Time"
        refused={{ "8": true, "9": true, "13": true }}
        reasonId="refused-hours"
      />
      <p id="refused-hours" className="t-caption">
        Times that have passed cannot be chosen.
      </p>
    </div>
  );
}

// Options the reader may not choose now stay in view, struck through, and
// every one points at the single sentence that says why.
export const RefusedOption: Story = {
  render: () => <RefusedDemo />,
  play: async ({ canvasElement }) => {
    const [first] = canvasElement.querySelectorAll("button");
    await expect(first?.disabled).toBe(true);
    await expect(first?.getAttribute("aria-describedby")).toBe("refused-hours");
  },
};

const PRESETS = ["readDraft", "full", "custom"] as const;
type Preset = (typeof PRESETS)[number];
const PRESET_LABELS: Record<Preset, string> = {
  readDraft: "Read and draft",
  full: "Full assistant",
  custom: "Pick your own",
};

const SECTIONS = ["overview", "research", "tasks"] as const;
type Section = (typeof SECTIONS)[number];
const SECTION_LABELS: Record<Section, string> = {
  overview: "Overview",
  research: "Data and tools",
  tasks: "Open tasks",
};

function NarrowDemo() {
  const [preset, setPreset] = useState<Preset>("readDraft");
  const [section, setSection] = useState<Section>("overview");
  return (
    <div style={stack}>
      <SegmentedControl
        options={PRESETS}
        value={preset}
        onChange={setPreset}
        labels={PRESET_LABELS}
        label="Access"
      />
      <SegmentedControl
        options={SECTIONS}
        value={section}
        onChange={setSection}
        labels={SECTION_LABELS}
        counts={{ research: 12, tasks: 3 }}
        marks={{ research: true }}
        label="Record sections"
      />
    </div>
  );
}

async function holdsOneRow(canvasElement: HTMLElement) {
  for (const strip of canvasElement.querySelectorAll(".segmented")) {
    const frame = strip.getBoundingClientRect();
    const segments = [...strip.querySelectorAll("button")].map((segment) => ({
      box: segment.getBoundingClientRect(),
      clipped: segment.scrollWidth > segment.clientWidth,
    }));
    for (const { box, clipped } of segments) {
      await expect(box.top).toBe(segments[0]?.box.top);
      await expect(box.right).toBeLessThanOrEqual(frame.right);
      await expect(clipped).toBe(false);
    }
  }
}

// Too narrow for the labels on one line: each label wraps inside its own
// segment, and the count stays beside the words it counts.
export const NarrowContainer: Story = {
  render: () => (
    <div style={{ inlineSize: "300px" }}>
      <NarrowDemo />
    </div>
  ),
  play: async ({ canvasElement }) => {
    await holdsOneRow(canvasElement);
    const presets = canvasElement.querySelector(".segmented");
    const widths = [...(presets?.children ?? [])].map((segment) =>
      Math.round(segment.getBoundingClientRect().width),
    );
    await expect(new Set(widths).size).toBe(1);
  },
};

export const NarrowContainerDark: Story = {
  ...NarrowContainer,
  globals: { theme: "dark" },
};

// A card on a 390px phone, too narrow for the presets on one line.
export const AtPhoneWidth: Story = {
  render: () => (
    <div style={{ padding: "var(--padCard)" }}>
      <Card as="div">
        <NarrowDemo />
      </Card>
    </div>
  ),
  play: async ({ canvasElement }) => holdsOneRow(canvasElement),
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
};

const STATUSES = ["new", "contacted", "engaged"] as const;
type Status = (typeof STATUSES)[number];

function GermanStatusDemo() {
  const t = useT();
  const [status, setStatus] = useState<Status>("new");
  return (
    <SegmentedControl
      options={STATUSES}
      value={status}
      onChange={setStatus}
      labels={{
        new: t("lead.status.new"),
        contacted: t("lead.status.contacted"),
        engaged: t("lead.status.engaged"),
      }}
      counts={{ new: 0, contacted: 0, engaged: 0 }}
      label="Status"
    />
  );
}

// How far each row of segments falls short of the track's content box.
function rowShortfalls(strip: Element): number[] {
  const style = getComputedStyle(strip);
  const content =
    strip.clientWidth -
    Number.parseFloat(style.paddingLeft) -
    Number.parseFloat(style.paddingRight);
  const gap = Number.parseFloat(style.columnGap) || 0;
  const rows = new Map<number, number[]>();
  for (const segment of strip.querySelectorAll("button")) {
    const { top, width } = segment.getBoundingClientRect();
    rows.set(Math.round(top), [...(rows.get(Math.round(top)) ?? []), width]);
  }
  return [...rows.values()].map(
    (widths) =>
      content -
      widths.reduce((sum, width) => sum + width, 0) -
      gap * (widths.length - 1),
  );
}

// The German status words with their counts cannot share one row at 320px.
// Each row the strip falls to is filled by its own segments.
export const GermanWrap: Story = {
  render: () => (
    <StoryProviders locale="de">
      <div style={{ inlineSize: "320px" }}>
        <GermanStatusDemo />
      </div>
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    const strip = canvasElement.querySelector(".segmented");
    await expect(strip).not.toBeNull();
    if (!strip) return;
    await waitFor(() => expect(rowShortfalls(strip).length).toBe(2));
    await waitFor(async () => {
      for (const shortfall of rowShortfalls(strip)) {
        await expect(Math.abs(shortfall)).toBeLessThanOrEqual(1);
      }
    });
  },
};
