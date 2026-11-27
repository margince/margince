// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { Pager } from "./pager";
import "./listtable.css";

// The footer of a list. What the strip of page numbers and the range under it
// say in each state of what the caller knows about the list's length.
const meta: Meta<typeof Pager> = {
  title: "Components/Text and data display/Pager",
  component: Pager,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => (
      <LocaleProvider initial="en">
        <Story />
      </LocaleProvider>
    ),
  ],
};
export default meta;

type Story = StoryObj<typeof Pager>;

const PER_PAGE = 25;

// The pager of a list of `total` rows on page `current`, as a list hands it.
function onPage(current: number, total: number, more = false) {
  return {
    current,
    lastPage: Math.ceil(total / PER_PAGE),
    perPage: PER_PAGE,
    range: {
      first: (current - 1) * PER_PAGE + 1,
      last: Math.min(current * PER_PAGE, total),
      total,
      more,
    },
    onGoto: () => undefined,
    onPerPage: () => undefined,
  };
}

const COUNTED = 1204;

// "1 2 3 4 5 … 49": the server counted the list, so the last page is numbered
// though only the first two are in hand.
export const Counted: Story = {
  args: { ...onPage(1, COUNTED), hasMore: true, endKnown: true },
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByText("1-25 of 1,204"),
    ).toBeVisible();
  },
};

// "1 … 4 5 6 … 49".
export const CountedMiddle: Story = {
  args: { ...onPage(5, COUNTED), hasMore: true, endKnown: true },
};

// "1 … 45 46 47 48 49", and Next is off.
export const CountedLast: Story = {
  args: { ...onPage(49, COUNTED), hasMore: true, endKnown: true },
};

// The reader pressed the last page; its number waits while the rows are read.
export const AskedPage: Story = {
  args: { ...onPage(2, COUNTED), hasMore: true, endKnown: true, asked: 49 },
};

// No count: the pages in hand, then a gap, since more follow.
export const NotCounted: Story = {
  args: { ...onPage(2, 50, true), hasMore: true },
};

// Every row is in hand, so the pages are all there are.
export const EveryRowInHand: Story = {
  args: { ...onPage(4, 220), hasMore: false },
};

// Seven pages or fewer: every page, no gap.
export const ThreePages: Story = {
  args: { ...onPage(2, 70), hasMore: false },
};

// A phone: five slots and chevrons named Previous and Next, edge to edge. Under
// the strip sit the range and a page size that shows its number alone.
// `uat-phone` drives the capture gate to 390px.
export const OnAPhone: Story = {
  name: "phone, five slots and chevrons",
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  args: { ...onPage(5, COUNTED), hasMore: true, endKnown: true },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("button", { name: "Previous" }).querySelector("svg"),
    ).not.toBeNull();
    await expect(
      canvas.getByRole("combobox", { name: "Rows per page" }),
    ).toHaveTextContent(/^25$/);
    await expectStacked(canvasElement);
    const strip = box(canvasElement, ".lt-pager");
    const row = box(canvasElement, ".lt-footrow");
    await expect(Math.abs(strip.width - row.width)).toBeLessThan(1);
  },
};

// A tablet: the short strip of a phone, centred, with the full page size.
// `uat-tablet` drives the capture gate to 768px.
export const OnATablet: Story = {
  name: "tablet, five slots and chevrons",
  globals: { viewport: { value: "tablet" } },
  tags: ["uat-tablet"],
  args: { ...onPage(5, COUNTED), hasMore: true, endKnown: true },
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await expect(
      canvas.getByRole("button", { name: "Next" }).querySelector("svg"),
    ).not.toBeNull();
    await expect(canvasElement.querySelectorAll(".lt-pager > *")).toHaveLength(
      7,
    );
    await expect(
      canvas.getByRole("combobox", { name: "Rows per page" }),
    ).toHaveTextContent("25 per page");
  },
};

const box = (canvasElement: HTMLElement, selector: string) => {
  const found = canvasElement.querySelector(selector);
  if (!found) {
    throw new Error(`no ${selector} in the story`);
  }
  return found.getBoundingClientRect();
};

// The strip on its own row; the range at the start and the page size at the
// end of the row under it.
async function expectStacked(canvasElement: HTMLElement) {
  const strip = box(canvasElement, ".lt-pager");
  const range = box(canvasElement, ".lt-range");
  const size = box(canvasElement, ".lt-perpage");
  const row = box(canvasElement, ".lt-footrow");
  await expect(range.top).toBeGreaterThanOrEqual(strip.bottom);
  await expect(Math.abs(range.left - row.left)).toBeLessThan(1);
  await expect(Math.abs(size.right - row.right)).toBeLessThan(1);
}

// A wide foot: one row, and the strip centred on the foot itself, whatever the
// widths of the range and the page size beside it.
export const WideFoot: Story = {
  args: { ...onPage(49, COUNTED), hasMore: true, endKnown: true },
  render: (args) => (
    <div style={{ maxWidth: 1100 }}>
      <Pager {...args} />
    </div>
  ),
  play: async ({ canvasElement }) => {
    const strip = box(canvasElement, ".lt-pager");
    const foot = box(canvasElement, ".lt-foot");
    const centre = (rect: DOMRect) => (rect.left + rect.right) / 2;
    await expect(Math.abs(centre(strip) - centre(foot))).toBeLessThan(1);
    await expect(box(canvasElement, ".lt-range").top).toBeLessThan(
      strip.bottom,
    );
  },
};

// A foot too narrow for that row on a wide screen: the stacked layout a
// phone uses, with seven slots and the words.
export const NarrowFoot: Story = {
  args: { ...onPage(49, COUNTED), hasMore: true, endKnown: true },
  render: (args) => (
    <div style={{ maxWidth: 560 }}>
      <Pager {...args} />
    </div>
  ),
  play: async ({ canvasElement }) => {
    await expect(
      within(canvasElement).getByRole("button", { name: "Next" }),
    ).toHaveTextContent("Next");
    await expectStacked(canvasElement);
  },
};
