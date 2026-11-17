// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useState } from "react";
import { expect, userEvent, waitFor, within } from "storybook/test";
import { LocaleProvider } from "../i18n";
import { Button } from "./atoms";
import { RecordTabs } from "./recordtabs";

// RecordTabs: the strip that chooses which body of a record is open, drawn as
// a rule with the current tab underlined rather than as a pill. Three things
// beside the ordinary counted row are worth seeing on their own: a tab that is
// not a list of things and so carries no count at all, and a tab carrying the
// unread mark — something waiting behind it that nobody has taken up.
const meta: Meta<typeof RecordTabs> = {
  title: "Components/Navigation/Record tabs",
  component: RecordTabs,
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
type Story = StoryObj<typeof RecordTabs>;

type Body = "overview" | "contacts" | "activity" | "brief";

const LABELS: Record<Body, string> = {
  overview: "Overview",
  contacts: "Contacts",
  activity: "Activity",
  brief: "Brief",
};

// Live, so the underline can be judged moving to the tab the reader picked.
function Live(props: Readonly<{ initial: Body }>) {
  const [value, setValue] = useState<Body>(props.initial);
  return (
    <RecordTabs
      label="Record"
      options={["overview", "contacts", "activity", "brief"]}
      value={value}
      onChange={setValue}
      labels={LABELS}
      counts={{ contacts: 10, activity: 27 }}
      marks={{ brief: true }}
    />
  );
}

// The ordinary row: two tabs counted, `overview` and `brief` carrying none —
// `overview` because it is a summary rather than a list, `brief` because it is
// a document. `brief` also carries the unread mark, so the dot and a missing
// count sit on the same tab without being confused for each other.
export const Mixed: Story = {
  render: () => <Live initial="overview" />,
};

// The tab with no count is the current one — being selected does not require a
// figure any more than it requires one on the row above.
export const SelectedTabHasNoCount: Story = {
  render: () => <Live initial="brief" />,
};

// A record read in ONE body. The lone tab is a LABEL for the page the reader
// is already on — no press, no handler — and what earns the row is the control
// riding at its end, which stands where every other record page keeps it.
export const Single: Story = {
  render: () => (
    <RecordTabs
      label="Record"
      options={["overview"]}
      value="overview"
      labels={LABELS}
      trailing={<Button aria-pressed={false}>Details</Button>}
    />
  ),
};

// The lone label squeezed on a phone: it never becomes a scroll box with nothing
// a keyboard can reach. The trailing control wraps under it, and a label wider
// than the row wraps its own words.
export const SingleSqueezed: Story = {
  tags: ["uat-phone"],
  render: () => (
    <RecordTabs
      label="Record"
      options={["overview"]}
      value="overview"
      labels={{
        ...LABELS,
        overview: "Overview of the account and everything we know about it",
      }}
      trailing={<Button aria-pressed={false}>Details</Button>}
    />
  ),
  play: async ({ canvasElement }) => {
    const strip = canvasElement.querySelector(".recordtabs-strip");
    const tab = within(canvasElement).getByText(/^Overview of the account/);
    const details = within(canvasElement).getByRole("button", {
      name: "Details",
    });
    if (!(strip instanceof HTMLElement)) throw new Error("no tab strip drawn");
    await expect(strip.scrollWidth).toBeLessThanOrEqual(strip.clientWidth);
    await expect(details.getBoundingClientRect().top).toBeGreaterThanOrEqual(
      tab.getBoundingClientRect().bottom,
    );
  },
};

// The row's far end carries the control that opens the record's details
// column. It sits off the strip's own scroll, so it stays in view however
// many tabs there are, and reads as the page's control rather than a tab.
export const WithTrailingControl: Story = {
  render: () => (
    <RecordTabs
      label="Record"
      options={["overview", "contacts", "activity", "brief"]}
      value="overview"
      onChange={() => undefined}
      labels={LABELS}
      counts={{ contacts: 10, activity: 27 }}
      trailing={<Button aria-pressed={false}>Details</Button>}
    />
  ),
};

// A trailing control too wide to leave room for the open tab and More beside
// it. It wraps under the strip, so the strip keeps a row of its own to fold in.
export const FoldsAboveAWideTrailingControl: Story = {
  tags: ["uat-phone"],
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  render: () => (
    <RecordTabs
      label="Analytics"
      options={["performance", "outcomes", "delivery", "pipeline", "team"]}
      value="performance"
      onChange={() => undefined}
      labels={ANALYTICS_LABELS}
      trailing={
        <>
          <Button>Reports</Button>
          <Button>Setup and sharing</Button>
        </>
      }
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await waitFor(() =>
      expect(
        canvas.getByRole("button", { name: "More sections" }),
      ).toBeVisible(),
    );
    const strip = canvasElement.querySelector(".recordtabs-strip");
    if (!(strip instanceof HTMLElement)) throw new Error("no tab strip drawn");
    await waitFor(() =>
      expect(strip.scrollWidth).toBeLessThanOrEqual(strip.clientWidth),
    );
    const reports = canvas.getByRole("button", { name: "Reports" });
    await expect(reports.getBoundingClientRect().top).toBeGreaterThanOrEqual(
      strip.getBoundingClientRect().bottom,
    );
  },
};

// Analytics' own tabs at 390px: more than the phone holds. The tabs after the
// last one that fits fold behind More, and the open tab stays on the row even
// when it comes last.
export const FoldsAtPhoneWidth: Story = {
  tags: ["uat-phone"],
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  render: () => (
    <RecordTabs
      label="Analytics"
      options={["performance", "outcomes", "delivery", "pipeline", "team"]}
      value="team"
      onChange={() => undefined}
      labels={{
        performance: "Performance",
        outcomes: "My outcomes",
        delivery: "Delivery",
        pipeline: "Pipeline",
        team: "Team",
      }}
    />
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await waitFor(() =>
      expect(
        canvas.getByRole("button", { name: "More sections" }),
      ).toBeVisible(),
    );
    await expect(canvas.getByRole("button", { name: "Team" })).toBeVisible();
    const strip = canvasElement.querySelector(".recordtabs-strip");
    if (!(strip instanceof HTMLElement)) throw new Error("no tab strip drawn");
    // The strip's scroll width settles a few frames after More shows.
    await waitFor(() =>
      expect(strip.scrollWidth).toBeLessThanOrEqual(strip.clientWidth),
    );
  },
};

const ANALYTICS_LABELS = {
  performance: "Performance",
  outcomes: "My outcomes",
  delivery: "Delivery",
  pipeline: "Pipeline",
  team: "Team",
};

// The folded tabs as the menu lists them, opened from "More".
export const MoreMenuOpen: Story = {
  tags: ["uat-phone"],
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  render: () => (
    <RecordTabs
      label="Analytics"
      options={["performance", "outcomes", "delivery", "pipeline", "team"]}
      value="performance"
      onChange={() => undefined}
      labels={ANALYTICS_LABELS}
    />
  ),
  play: async ({ canvasElement }) => {
    const more = await within(canvasElement).findByRole("button", {
      name: "More sections",
    });
    await waitFor(() => expect(more).toBeVisible());
    await userEvent.click(more);
    const page = within(canvasElement.ownerDocument.body);
    await expect(
      await page.findByRole("button", { name: "Team" }),
    ).toBeVisible();
  },
};

// The row narrows until tabs fold, then widens until they all return.
export const FoldsAndReturnsWithWidth: Story = {
  render: () => (
    <div data-testid="frame" style={{ inlineSize: "100%" }}>
      <RecordTabs
        label="Analytics"
        options={["performance", "outcomes", "delivery", "pipeline", "team"]}
        value="team"
        onChange={() => undefined}
        labels={ANALYTICS_LABELS}
      />
    </div>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const frame = canvas.getByTestId("frame");
    frame.style.inlineSize = "260px";
    await waitFor(() =>
      expect(
        canvas.getByRole("button", { name: "More sections" }),
      ).toBeVisible(),
    );
    frame.style.inlineSize = "100%";
    // Performance leads the row and never folds, so the wait is on a tab
    // that did fold coming back.
    await waitFor(() =>
      expect(canvas.getByRole("button", { name: "Pipeline" })).toBeVisible(),
    );
    await waitFor(() =>
      expect(
        canvas.queryByRole("button", { name: "More sections" }),
      ).toBeNull(),
    );
  },
};

// A keyboard reader can press a tab while the strip is out of view. The pane
// then scrolls up just far enough to show the strip, and no further.
export const PressedOutOfView: Story = {
  render: () => <InPane />,
  play: async ({ canvasElement }) => {
    const pane = within(canvasElement).getByTestId("pane");
    const row = canvasElement.querySelector(".recordtabs-row");
    if (!(row instanceof HTMLElement)) throw new Error("no tab strip drawn");
    const stripTop = () =>
      row.getBoundingClientRect().top - pane.getBoundingClientRect().top;
    pane.scrollTop += stripTop() + 200;
    await waitFor(() => expect(stripTop()).toBeLessThan(-150));
    within(canvasElement)
      .getByRole("button", { name: /Activity/ })
      .click();
    await waitFor(() => expect(Math.abs(stripTop())).toBeLessThanOrEqual(1));
    await expect(pane.scrollTop).toBeGreaterThan(0);
  },
};

function InPane() {
  return (
    <div
      data-testid="pane"
      className="scroll"
      style={{ display: "block", blockSize: "24rem", overflowY: "auto" }}
    >
      <div style={{ blockSize: "16rem" }}>Record header</div>
      <Live initial="overview" />
      <div style={{ blockSize: "60rem" }}>Tab body</div>
    </div>
  );
}
