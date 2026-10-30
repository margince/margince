// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { LocaleProvider } from "../i18n";
import { ScrollSentinel } from "./scrollsentinel";

// The end of a list that loads as the reader scrolls, in each state it can
// show. Flip the Theme control: the line is secondary ink in both themes.
const meta: Meta = {
  title: "Components/Loading/Scroll sentinel",
  component: ScrollSentinel,
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
type Story = StoryObj;

const idle = { list: "story", onLoadMore: () => undefined } as const;

export const Loading: Story = {
  name: "A batch in flight",
  render: () => <ScrollSentinel {...idle} cursor="c1" loading failed={false} />,
};

export const Failed: Story = {
  name: "A batch that did not arrive",
  render: () => <ScrollSentinel {...idle} cursor="c1" loading={false} failed />,
};

export const End: Story = {
  name: "Every batch loaded",
  render: () => (
    <ScrollSentinel
      {...idle}
      cursor={undefined}
      loading={false}
      failed={false}
      end="All decisions loaded"
    />
  ),
};

// A list of forty rows in a short scroll box, loaded ten at a time.
function Growing() {
  const [rows, setRows] = useState(10);
  const [loading, setLoading] = useState(false);
  const more = () => {
    setLoading(true);
    setTimeout(() => {
      setRows((count) => count + 10);
      setLoading(false);
    }, 400);
  };
  return (
    <div style={{ height: 240, overflow: "auto" }}>
      <ol>
        {Array.from({ length: rows }, (_, at) => `Row ${at + 1}`).map(
          (label) => (
            <li key={label}>{label}</li>
          ),
        )}
      </ol>
      <ScrollSentinel
        list="growing"
        cursor={rows < 40 ? String(rows) : undefined}
        loading={loading}
        failed={false}
        onLoadMore={more}
        end="All 40 rows loaded"
      />
    </div>
  );
}

export const Scrolling: Story = {
  name: "Loading as the box scrolls",
  render: () => <Growing />,
};
