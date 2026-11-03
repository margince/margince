// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { expect, userEvent, within } from "storybook/test";
import { CallStartProvider } from "../lib/callstart";
import {
  callMessengersRoute,
  installFetchStub,
  StoryProviders,
} from "../testing/story-utils";
import { CallAction, linkVerb } from "./callaction";
import { Fact, RecordFacts } from "./recordfacts";

// Each appearance with a number that dials, one `telUri` refuses, and none.
// The shell's call log host is stood in for by a start that opens nothing.
const meta: Meta<typeof CallAction> = {
  title: "Components/Navigation/Call action",
  component: CallAction,
  parameters: { layout: "padded" },
  decorators: [
    (Story) => {
      installFetchStub({
        "GET /installation/settings": callMessengersRoute(),
      });
      return (
        <StoryProviders>
          <CallStartProvider start={() => {}}>
            <Story />
          </CallStartProvider>
        </StoryProviders>
      );
    },
  ],
};
export default meta;

type Story = StoryObj<typeof CallAction>;

const DIALS = "+49 30 1234 5678";
const REFUSED = "ask reception";
const dana = { entityType: "contact", entityId: "p-1" } as const;

const row = { display: "flex", gap: "var(--space-4)", alignItems: "center" };

// The chats fold behind the glyph after the number; a reader opens it first.
async function openChats(canvasElement: HTMLElement) {
  await userEvent.click(
    await within(canvasElement).findByRole("button", { name: "Open a chat" }),
  );
}

// A number with its country code offers its chats; a national one only dials.
export const Value: Story = {
  render: () => (
    <div style={row}>
      <CallAction appearance="value" record={dana} number={DIALS} />
      <CallAction appearance="value" record={dana} number="030 1234 5678" />
      <CallAction appearance="value" record={dana} number={REFUSED} />
    </div>
  ),
  play: async ({ canvasElement }) => {
    await openChats(canvasElement);
    await expect(
      await within(document.body).findAllByRole("link", {
        name: /^Open chat in /,
      }),
    ).toHaveLength(3);
  },
};

export const ValueWithTelegramOff: Story = {
  render: () => {
    installFetchStub({
      "GET /installation/settings": callMessengersRoute({
        call_telegram: false,
      }),
    });
    return <CallAction appearance="value" record={dana} number={DIALS} />;
  },
  play: async ({ canvasElement }) => {
    await openChats(canvasElement);
    const page = within(document.body);
    await page.findByRole("link", { name: "Open chat in Zalo" });
    await expect(
      page.queryByRole("link", { name: "Open chat in Telegram" }),
    ).toBeNull();
  },
};

const LONG = "+849123456789012";

/**
 * The header's facts strip at 390px: the chat glyph after a long number stays
 * inside the edge. `uat-phone` makes the capture gate render it at 390px.
 */
export const ValueAtPhoneWidth: Story = {
  render: () => (
    <div style={{ padding: "var(--padCard)" }}>
      <RecordFacts>
        <Fact label="Email">
          dana.brandt-oberhausen@brandt-automotive.example
        </Fact>
        <Fact label="Phone">
          <CallAction appearance="value" record={dana} number={LONG} />
        </Fact>
      </RecordFacts>
    </div>
  ),
  parameters: { layout: "fullscreen" },
  globals: { viewport: { value: "phone" } },
  tags: ["uat-phone"],
  play: async ({ canvasElement }) => {
    const canvas = within(canvasElement);
    const chat = await canvas.findByRole("button", { name: "Open a chat" });
    const number = canvas.getByRole("link", { name: LONG });
    const page = canvasElement.getBoundingClientRect();
    for (const control of [...canvas.getAllByRole("link"), chat]) {
      await expect(control.getBoundingClientRect().right).toBeLessThanOrEqual(
        page.right + 0.5,
      );
    }
    await expect(canvasElement.scrollWidth).toBeLessThanOrEqual(
      canvasElement.clientWidth,
    );
    await expect(chat.getBoundingClientRect().left).toBeGreaterThanOrEqual(
      number.getBoundingClientRect().left - 0.5,
    );
  },
};

// A refused number and no number draw nothing, so only the first shows.
export const ButtonForm: Story = {
  name: "Button",
  render: () => (
    <div style={row}>
      <CallAction
        appearance="button"
        record={dana}
        number={DIALS}
        label="Call"
      />
      <CallAction
        appearance="button"
        record={dana}
        number={REFUSED}
        label="Call"
      />
      <CallAction
        appearance="button"
        record={dana}
        number={undefined}
        label="Call"
      />
    </div>
  ),
};

export const Icon: Story = {
  render: () => (
    <div style={row}>
      <CallAction
        appearance="icon"
        record={dana}
        number={DIALS}
        label="Call"
        reason="No phone number on this contact."
      />
      <CallAction
        appearance="icon"
        record={dana}
        number={REFUSED}
        label="Call"
        reason="No phone number on this contact."
      />
      <CallAction
        appearance="icon"
        record={dana}
        number={undefined}
        label="Call"
        reason="No phone number on this contact."
      />
    </div>
  ),
};

// The page's own sentence refuses the call even though a number is on file.
export const IconRefusedByPage: Story = {
  render: () => (
    <div>
      <CallAction
        appearance="icon"
        record={dana}
        number={DIALS}
        label="Call"
        reason="No phone number on this contact."
        reasonId="callaction-archived"
      />
      <p id="callaction-archived" className="t-caption">
        This contact is archived and takes no changes.
      </p>
    </div>
  ),
};

export const RecordLink: Story = {
  render: () =>
    linkVerb(
      "#/contacts/01a00000-0000-7000-8000-000000000001",
      null,
      "Open contact",
    ),
};
