// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import type { components } from "../api/schema";
import { LocaleProvider } from "../i18n";
import { SendPermission } from "./sendpermission";

// What the engine decided about a message, said where the rep is writing it.
//
// One component for every surface that stages a send: the composer, the
// scheduled-send queue and the approval card. Three states and a rep sees
// exactly one, chosen by the engine's answer. The allowed state renders
// NOTHING, which is why it has no story: the overwhelming majority of sends
// should cost no attention at all.
const meta: Meta = {
  title: "Patterns/Send permission",
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

type Preview = components["schemas"]["SendAuthorizationPreview"];
type Recipient = components["schemas"]["SendAuthorizationPreviewRecipient"];

function refused(over: Partial<Recipient>): Preview {
  return {
    allowed: false,
    recipients: [
      {
        address: "anna@example.test",
        verdict: "deny",
        reason_code: "no_compatible_evidence",
        decided_by: "machine",
        would_refuse: true,
        can_be_overruled: false,
        ...over,
      },
    ],
  };
}

/** The subject's own act. No control, and the sentence says whose decision it is. */
export const RefusedBySubject: Story = {
  render: () => (
    <SendPermission
      preview={refused({
        reason_code: "marketing_objection",
        decided_by: "subject",
      })}
    />
  ),
};

/** The engine's own reading, and still absolute: a dead mailbox is nobody's to lift. */
export const RefusedByTheRecord: Story = {
  render: () => (
    <SendPermission
      preview={refused({ reason_code: "hard_bounce", decided_by: "machine" })}
    />
  ),
};

/**
 * The engine has no record, and this surface can take the rep's answer. The
 * ONLY state that offers a control.
 */
export const UnprovenWithOverride: Story = {
  render: () => (
    <SendPermission
      preview={refused({ can_be_overruled: true })}
      onOverride={() => undefined}
    />
  ),
};

/**
 * The same state on a surface that cannot take an answer — an approval card,
 * or a composer before the override exists. It explains, and says what happens
 * to the message as it stands, rather than drawing a button that does nothing.
 */
export const UnprovenReadOnly: Story = {
  render: () => (
    <SendPermission preview={refused({ can_be_overruled: true })} />
  ),
};

/** The question did not arrive. Said out loud, because silence reads as yes. */
export const Unanswered: Story = {
  render: () => <SendPermission preview={undefined} unanswered />,
};
