// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { useEffect } from "react";
import { expect, within } from "storybook/test";
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

function RecordFrame({
  queue,
  locale = "en",
}: Readonly<{ queue: WorklistItem[]; locale?: Locale }>) {
  useEffect(() => {
    const before = window.location.hash;
    installFetchStub({
      "GET /worklist": () => jsonResponse(readingsDay({}, queue)),
    });
    window.location.hash = "#/contacts/c-weber?filter=tasks&from=worklist";
    announceAddressChanged();
    return () => {
      window.location.hash = before;
      announceAddressChanged();
    };
  }, [queue]);
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
  render: () => <RecordFrame queue={ahead} />,
};
export default meta;
type Story = StoryObj<typeof NextTaskButton>;

/** Another record has an open row: the press opens it. */
export const NextTaskAhead: Story = {
  play: async ({ canvasElement }) => {
    const button = await within(canvasElement).findByRole("button", {
      name: en["brief.queue.next"],
    });
    await expect(button).toBeEnabled();
  },
};

/** Only this record is left in the list: the button says so and refuses. */
export const WorklistDone: Story = {
  render: () => <RecordFrame queue={[weber]} />,
  play: async ({ canvasElement }) => {
    const button = await within(canvasElement).findByRole("button", {
      name: en["brief.queue.nextDone"],
    });
    await expect(button).toBeDisabled();
  },
};

/** The longest label, at phone width: it wraps under the bar, never off it. */
export const GermanPhone: Story = {
  tags: ["uat-phone"],
  render: () => <RecordFrame queue={ahead} locale="de" />,
  play: async ({ canvasElement }) => {
    await within(canvasElement).findByRole("button", {
      name: de["brief.queue.next"],
    });
    await expect(canvasElement.scrollWidth).toBeLessThanOrEqual(
      canvasElement.clientWidth,
    );
  },
};
