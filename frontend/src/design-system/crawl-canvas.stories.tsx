// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { CountUp } from "./countup";
import { CrawlCanvas, type CrawlPage } from "./crawl-canvas";

// The read's picture, which is the whole screen while a crawl runs.
//
// WATCH IT RATHER THAN READ THE CAPTURE. The graph builds over about five
// seconds at one page every 400ms and then loops, so a still frame taken at any
// fixed moment is mid-build by construction. What the capture IS good for is the
// two themes: the ink is `--ai` and the resting nodes are `--textTertiary`, and
// both move with the theme control.
const meta: Meta<typeof CrawlCanvas> = {
  title: "Onboarding/Crawl canvas",
  component: CrawlCanvas,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof CrawlCanvas>;

// A real site's shape: a root, the pages it links to, and what those link to.
const PAGES: readonly CrawlPage[] = [
  { path: "/", note: "read" },
  { path: "/product", note: "read" },
  { path: "/pricing", note: "read" },
  { path: "/customers", note: "read" },
  { path: "/about", note: "read" },
  { path: "/customers/atlas", note: "read" },
  { path: "/careers", note: "read" },
  { path: "/blog", note: "read" },
  { path: "/blog/why-crm", note: "read" },
  { path: "/security", note: "read" },
  { path: "/imprint", note: "read" },
  { path: "/contact", note: "read" },
];

export const AWholeSite: Story = {
  args: { pages: PAGES, label: "12 pages read from margince.com" },
};

// The first moments, when the root is all there is. The picture has to be
// honest this early too: one node, no edges, and nothing drawn ahead of it.
export const JustTheRoot: Story = {
  args: { pages: PAGES.slice(0, 1), label: "1 page read from margince.com" },
};

// A read that has reached nothing. Not an error, just the shape of the first
// second, and the reason the canvas must never draw a node it was not given.
export const NothingYet: Story = {
  args: { pages: [], label: "no pages read yet" },
};

// The picture with the figures it sits above, which is how the screen actually
// composes it: the graph is the pleasant version of a fact the numbers state.
export const WithItsCounters: Story = {
  render: () => (
    <div style={{ display: "grid", gap: "var(--space-5)" }}>
      <CrawlCanvas pages={PAGES} label="12 pages read from margince.com" />
      <dl className="ob-scan-tally">
        <div>
          <dt>pages read</dt>
          <dd>
            <CountUp value={12} locale="en-GB" />
          </dd>
        </div>
        <div>
          <dt>facts found</dt>
          <dd>
            <CountUp value={146} locale="en-GB" />
          </dd>
        </div>
      </dl>
    </div>
  ),
};
