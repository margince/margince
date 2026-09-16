// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ReactNode, useState } from "react";
import { expect, userEvent, within } from "storybook/test";
import { InlineText } from "../design-system/inlinetext";
import { Panel, PanelBody } from "../design-system/panel";
import { RecordTabs } from "../design-system/recordtabs";
import { RecordView } from "../design-system/recordview";
import { LocaleProvider } from "../i18n";
import { en } from "../i18n/en";
import { installFetchStub } from "../screens/story-utils";
import { PageAsideProvider, PageAsideToggle, usePageAside } from "./pageaside";
import { UnsavedGuard, useUnsavedGuard } from "./unsaved";

// The record's details pane, in the three states a reader can put it in: open
// beside the work under the tab row, folded away, and absent because the
// screen offers none.
//
// Drawn inside a `RecordView`, because the pane IS that view's aside slot: a
// story that floated the cards on their own would be reviewing a geometry the
// product never draws. The fetch stub routes nothing, because nothing on this
// surface asks the server anything and the empty stub is what says so.
//
// READ THIS AT TWO WIDTHS. Under 1200px PageZones stacks the pane under the
// work column rather than beside it; the capture gate's canvas is 1024px wide,
// so its screenshots are that layout, and the 300px column beside the work is
// what `pnpm storybook` shows on any display past 1200.

const meta: Meta<typeof PageAsideToggle> = {
  title: "Shell/Page aside",
  component: PageAsideToggle,
  parameters: { layout: "fullscreen" },
};
export default meta;
type Story = StoryObj<typeof PageAsideToggle>;

// Two of the company rail's own cards, in their empty state: real details-pane
// copy from the catalog rather than invented filler, so the pane is judged
// against the width and rhythm it actually carries.
function contextCards() {
  return (
    <>
      <Panel title={en["co.rail.deals.title"]}>
        <PanelBody>{en["co.rail.deals.empty"]}</PanelBody>
      </Panel>
      <Panel title={en["co.rail.contacts.title"]}>
        <PanelBody>{en["co.rail.contacts.empty"]}</PanelBody>
      </Panel>
    </>
  );
}

// A record screen's shape: it claims the pane, hands the view its content
// whether or not the pane is showing and says which, and carries the switch at
// the end of the tab row.
function Record() {
  const details = usePageAside();
  return (
    <RecordView
      name="Brandt Automotive GmbH"
      zone="UTC"
      tabs={
        <RecordTabs
          options={["overview", "history"]}
          value="overview"
          onChange={() => undefined}
          labels={{ overview: en["tab.overview"], history: en["tab.history"] }}
          trailing={<PageAsideToggle />}
        />
      }
      aside={contextCards()}
      asideOpen={details.open}
    >
      <Panel title={en["co.commercial.title"]}>
        <PanelBody>{en["co.work.noDeals"]}</PanelBody>
      </Panel>
    </RecordView>
  );
}

// A screen that offers no pane at all: it never claims one, so the tab row's
// switch has nothing to govern and draws nothing.
function PlainRecord() {
  return (
    <RecordView
      name="Brandt Automotive GmbH"
      zone="UTC"
      tabs={
        <RecordTabs
          options={["overview", "history"]}
          value="overview"
          onChange={() => undefined}
          labels={{ overview: en["tab.overview"], history: en["tab.history"] }}
          trailing={<PageAsideToggle />}
        />
      }
    >
      <Panel title={en["co.commercial.title"]}>
        <PanelBody>{en["co.work.noDeals"]}</PanelBody>
      </Panel>
    </RecordView>
  );
}

function page(screen: ReactNode, open: boolean) {
  return () => {
    installFetchStub({});
    return (
      <LocaleProvider initial="en">
        <PageAsideProvider open={open}>
          <div className="wrap">{screen}</div>
        </PageAsideProvider>
      </LocaleProvider>
    );
  };
}

/** The pane open beside the work, under the tab row, at its own 300px. */
export const Open: Story = { render: page(<Record />, true) };

/**
 * Folded away: the work takes the whole width and the switch reads "show".
 * The play flips it and asserts the pane arrives, and the switch says so.
 */
export const Folded: Story = {
  render: page(<Record />, false),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const user = userEvent.setup();
    // The switch renders nothing until the screen's `usePageAside` effect has
    // claimed the pane, which is a commit past mount: the query has to wait for
    // it rather than assume the first paint carried it. The pane is open by
    // default, so the first press folds it.
    await user.click(
      await canvas.findByRole("button", {
        name: en["record.panel.hideDetails"],
      }),
    );
    await expect(canvasElement.querySelector("aside")).toBeNull();
    await user.click(
      canvas.getByRole("button", { name: en["record.panel.showDetails"] }),
    );
    await expect(canvasElement.querySelector("aside")).not.toBeNull();
    await expect(
      canvas.getByRole("button", { name: en["record.panel.hideDetails"] }),
    ).toHaveAttribute("aria-pressed", "true");
  },
};

/** The screen offers no pane, so the switch is absent rather than present and
 *  inert — a switch for a pane that does not exist would be a control that
 *  does nothing. */
export const Empty: Story = { render: page(<PlainRecord />, true) };

function ControlledQueue() {
  const [open, setOpen] = useState(false);
  return (
    <LocaleProvider>
      <PageAsideProvider>
        <PageAsideToggle
          controlled={{
            open,
            labels: { show: "Show work queue", hide: "Hide work queue" },
            onToggle: () => setOpen(!open),
          }}
        />
      </PageAsideProvider>
    </LocaleProvider>
  );
}
export const WorkQueueControl: Story = {
  render: () => <ControlledQueue />,
  play: async ({ canvasElement }) => {
    const toggle = within(canvasElement).getByRole("button", {
      name: "Work queue",
    });
    await userEvent.click(toggle);
    await expect(toggle).toHaveAttribute("aria-pressed", "true");
  },
};

function ProtectedDetails() {
  const details = usePageAside();
  const [dirty, setDirty] = useState(false);
  useUnsavedGuard(dirty, "details");
  return (
    <>
      <PageAsideToggle />
      {details.open && (
        <InlineText
          label="Name"
          placeholder="Not set"
          value="Original"
          canEdit
          onDirtyChange={setDirty}
          onSave={async () => undefined}
        />
      )}
    </>
  );
}
export const DraftProtected: Story = {
  render: () => (
    <LocaleProvider initial="en">
      <UnsavedGuard address="record" onKeep={() => undefined}>
        {() => (
          <PageAsideProvider open>
            <ProtectedDetails />
          </PageAsideProvider>
        )}
      </UnsavedGuard>
    </LocaleProvider>
  ),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const user = userEvent.setup();
    await user.click(
      await canvas.findByRole("button", { name: "Change Name" }),
    );
    await expect(canvas.getByRole("button", { name: "Details" })).toBeEnabled();
    await user.type(canvas.getByRole("textbox", { name: "Name" }), " revised");
    await expect(
      canvas.getByRole("button", { name: "Details" }),
    ).toBeDisabled();
    await user.keyboard("{Escape}");
    await expect(canvas.getByRole("button", { name: "Details" })).toBeEnabled();
  },
};
