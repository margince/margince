// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { LocaleProvider } from "../i18n";
import type { TimelineFilters } from "./recordtimeline";
import { TimelineFilterBar } from "./timelinefilterbar";

const meta: Meta<typeof TimelineFilterBar> = {
  title: "Design System/Timeline filter bar",
  component: TimelineFilterBar,
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
type Story = StoryObj<typeof TimelineFilterBar>;

/**
 * Live, because the bar's own state is half its behaviour: the search box holds
 * a DRAFT that commits on Enter or on blur, and a filter reset arriving from
 * outside has to clear that draft — a box still showing the previous record's
 * word would claim a search that is not running.
 *
 * The dates are fixed rather than relative to today, so this frame reads the
 * same whenever it is opened.
 */
function Live(props: Readonly<{ initial: TimelineFilters }>) {
  const [filters, setFilters] = useState<TimelineFilters>(props.initial);
  return <TimelineFilterBar value={filters} onChange={setFilters} />;
}

/** Nothing narrowed: every kind, no words, no bounds. */
export const Unfiltered: Story = { render: () => <Live initial={{}} /> };

/** One kind. The dials are independent, so a kind and a range compose. */
export const ByKind: Story = {
  render: () => <Live initial={{ kind: "email" }} />,
};

/**
 * A search is running, and the bar SAYS what that costs: a text match answers
 * from a content-gated read, so a limited conversation the reader may not open
 * is simply absent from the matches. Silence would present what came back as
 * all there is.
 */
export const SearchingSaysWhatItOmits: Story = {
  render: () => <Live initial={{ q: "renewal" }} />,
};

/** A bounded window, with both ends set. Each input caps the other, so a range
 * cannot be inverted from inside the control. */
export const BoundedRange: Story = {
  render: () => (
    <Live initial={{ after: "2026-01-01", before: "2026-03-31" }} />
  ),
};

/** Everything at once, which is the widest the row gets before it wraps. */
export const FullyNarrowed: Story = {
  render: () => (
    <Live
      initial={{
        kind: "meeting",
        q: "pricing",
        after: "2026-01-01",
        before: "2026-03-31",
      }}
    />
  ),
};
