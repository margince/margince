// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { NextTaskSlot } from "../../components/nexttaskslot";
import { NextBar } from "../../design-system/nextbar";
import type { Locale } from "../../i18n";
import { de } from "../../i18n/de";
import { en } from "../../i18n/en";
import { announceAddressChanged } from "../../lib/router";
import { readingsDay, taskRow } from "../../testing/brief.fixtures";
import {
  installFetchStub,
  jsonResponse,
  StoryProviders,
} from "../../testing/story-utils";
import { NextTaskButton, NextTaskHost } from "./worklist.nexttask";
import type { WorklistItem } from "./worklist.queries";
import { WorklistReturnLink } from "./worklist.return";

// Next task as a record page draws it: through the shell's slot, at the next bar's end.

function onContact(id: string, title: string): WorklistItem {
  return {
    ...taskRow(`t-${id}`, title),
    subject: { type: "contact", id, label: title },
  };
}

// Before the first render: an address that lands after it remounts the button
// the play already holds.
function onRecordPage(queue: WorklistItem[], reading = false) {
  return () => {
    const before = window.location.hash;
    installFetchStub({
      "GET /worklist": () =>
        reading
          ? new Promise<Response>(() => {})
          : jsonResponse(readingsDay({}, queue)),
    });
    window.location.hash = "#/contacts/c-weber?filter=tasks&from=worklist";
    announceAddressChanged();
    return () => {
      window.location.hash = before;
      announceAddressChanged();
    };
  };
}

function RecordFrame({ locale = "en" }: Readonly<{ locale?: Locale }>) {
  return (
    <StoryProviders locale={locale}>
      <NextTaskHost>
        <WorklistReturnLink />
        <NextBar
          title="Send Dana the renewal paperwork"
          sub="Due 25 Jul"
          onward={<NextTaskSlot />}
        />
      </NextTaskHost>
    </StoryProviders>
  );
}

const weber = onContact("c-weber", "Call Weber");
const ahead = [weber, onContact("c-dana", "Send Dana the quote")];

const meta: Meta<typeof NextTaskButton> = {
  title: "Records/Record 360/Next task",
  component: NextTaskButton,
  parameters: { layout: "padded" },
  render: () => <RecordFrame />,
};
export default meta;
type Story = StoryObj<typeof NextTaskButton>;

/** At rest the button reads nothing and is always offered. */
export const NextTaskAhead: Story = {
  beforeEach: onRecordPage(ahead),
  play: async ({ canvasElement }) => {
    const button = await within(canvasElement).findByRole("button", {
      name: en["brief.queue.next"],
    });
    await expect(button).toBeEnabled();
  },
};

/** Pressed, while the list is read: busy, and a second press is swallowed. */
export const Reading: Story = {
  beforeEach: onRecordPage(ahead, true),
  play: async ({ canvasElement }) => {
    const button = await within(canvasElement).findByRole("button", {
      name: en["brief.queue.next"],
    });
    await userEvent.click(button);
    await expect(button).toHaveAttribute("aria-busy", "true");
  },
};

/** Pressed with only this record left on a record page: it says so and refuses. */
export const NoMoreRecords: Story = {
  beforeEach: onRecordPage([weber]),
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: en["brief.queue.next"] }),
    );
    const button = await canvas.findByRole("button", {
      name: en["brief.queue.nextNone"],
    });
    await expect(button).toBeDisabled();
  },
};

/** The longest label, at phone width: it wraps under the bar, never off it. */
export const GermanPhone: Story = {
  tags: ["uat-phone"],
  beforeEach: onRecordPage([weber]),
  render: () => <RecordFrame locale="de" />,
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    await userEvent.click(
      await canvas.findByRole("button", { name: de["brief.queue.next"] }),
    );
    await canvas.findByRole("button", { name: de["brief.queue.nextNone"] });
    await expect(canvasElement.scrollWidth).toBeLessThanOrEqual(
      canvasElement.clientWidth,
    );
  },
};
