// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { userEvent, within } from "storybook/test";
import type { components } from "../api/schema";
import { CoverageCard } from "./onboarding-live-panel";
import { StoryProviders } from "./story-utils";
import "./onboarding-live-panel.css";

// The coverage card, which is the one card in the dossier stack whose whole job
// is to name what a read did NOT get. Both states are worth seeing: a clean
// read says so in one line, and a stopped one lists the pages with the reason
// each carries — a URL alone would leave the reader decoding a path.

const meta: Meta = {
  title: "Onboarding/Coverage card",
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj;

type SitePage = components["schemas"]["CompanySiteReadPage"];

const CLEAN: readonly SitePage[] = [
  { url: "https://havbris.example", status: "fetched", kind: "home" },
  { url: "https://havbris.example/about", status: "fetched", kind: "about" },
];

const PARTIAL: readonly SitePage[] = [
  ...CLEAN,
  {
    url: "https://havbris.example/impressum",
    status: "failed",
    kind: "impressum",
    reason: "unreadable",
  },
  {
    url: "https://havbris.example/team",
    status: "skipped",
    kind: "team",
    reason: "page_cap",
  },
];

/** Nothing missed, so the card settles: a filled check and one sentence. */
export const CleanRead: Story = {
  render: () => (
    <StoryProviders>
      <CoverageCard pages={CLEAN} warnings={[]} />
    </StoryProviders>
  ),
};

/** Opened, because the rows are the point: each names the page and why. */
export const StoppedEarly: Story = {
  render: () => (
    <StoryProviders>
      <CoverageCard
        pages={PARTIAL}
        warnings={["The sitemap named 400 pages; the first 40 were read."]}
        stoppedReason="page_cap"
      />
    </StoryProviders>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    // Collapsed is the default, and the header carries the count that makes
    // that safe — so the story that is about the ROWS has to open it.
    await userEvent.click(
      await canvas.findByRole("button", { name: /What I read/ }),
    );
    await canvas.findByText(/400 pages/);
  },
};
