// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { LocaleProvider } from "../i18n";
import { Button } from "./atoms";
import {
  type CommunicationState,
  CommunicationStatus,
  CommunicationStatusLine,
} from "./communicationstatus";
import { FactList } from "./factlist";

const meta: Meta = {
  title: "Design System/Communication status",
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

// The six, side by side. This is the story that matters: the states have to be
// tellable apart by SHAPE, so a reader who takes in no colour still reads six
// different answers rather than one envelope in six inks.
const STATES: readonly {
  state: CommunicationState;
  label: string;
}[] = [
  { state: "context_only", label: "Checked for each message" },
  { state: "ready", label: "Ready to send" },
  { state: "attention", label: "Needs a look before sending" },
  { state: "restricted", label: "They asked us to stop marketing" },
  { state: "checking", label: "Checking…" },
  { state: "external", label: "Opens in your mail app" },
];

export const States: Story = {
  render: () => (
    <div
      style={{ display: "flex", gap: "var(--space-5)", alignItems: "center" }}
    >
      {STATES.map(({ state, label }) => (
        <CommunicationStatus
          key={state}
          state={state}
          scope="current_message"
          label={label}
          name={`Communication status for Anna: ${label}. View details.`}
        />
      ))}
    </div>
  ),
};

// The composer's footer form: the words beside the mark, because there is one
// message on screen and the reader is about to press send on it.
export const WithLabel: Story = {
  render: () => (
    <CommunicationStatusLine
      status={
        <CommunicationStatus
          state="attention"
          scope="current_message"
          showLabel
          label="No record of what this message is about"
          name="Communication status: no record of what this message is about. View details."
          detail={
            <FactList
              facts={[
                {
                  key: "anna",
                  term: "Anna Meier",
                  value: "No lawful basis on file",
                },
                { key: "checked", term: "Checked", value: "Just now" },
              ]}
            />
          }
        />
      }
    >
      <Button variant="ghost">Schedule</Button>
      <Button variant="primary">Send</Button>
    </CommunicationStatusLine>
  ),
};

// The dense form, in a row. The glyph is 16 and the touch target is still 44,
// which is why the row does not grow to fit it.
export const Dense16: Story = {
  render: () => (
    <div style={{ display: "grid", maxInlineSize: "420px" }}>
      {["Anna Meier", "Bruno Kraft", "Cem Yilmaz"].map((who, i) => (
        <div
          key={who}
          style={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            padding: "var(--space-2) 0",
          }}
        >
          <span>{who}</span>
          <CommunicationStatus
            state={i === 1 ? "restricted" : "context_only"}
            scope="contact_preferences"
            size={16}
            label={
              i === 1
                ? "They asked us to stop marketing"
                : "Checked for each message"
            }
            name={`Communication status for ${who}. View details.`}
          />
        </div>
      ))}
    </div>
  ),
};

// A message sent under a recorded exception. The mark stays RESTRICTED — the
// refusal was real and is still on the record — and the exception sits beside
// it rather than turning the mark green.
export const Exception: Story = {
  render: () => (
    <CommunicationStatus
      state="restricted"
      scope="historical_message"
      showLabel
      label="Assessment at send time: marketing objection"
      name="Communication status: marketing objection at send time. View details."
      exception="Sent with a recorded exception by Lars Jankowfsky"
    />
  ),
};

// A parked message, where the answer on screen is older than the message.
export const Freshness: Story = {
  render: () => (
    <CommunicationStatus
      state="ready"
      scope="current_message"
      showLabel
      label="Ready to send"
      name="Communication status: ready to send. View details."
      freshness="Checked now · Will check again before sending"
    />
  ),
};
