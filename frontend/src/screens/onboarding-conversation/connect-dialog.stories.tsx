// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Meta, StoryObj } from "@storybook/react-vite";
import { screen, within } from "storybook/test";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { useT } from "../../i18n";
import { StoryProviders } from "../story-utils";
import { ConnectDialog } from "./connect-dialog";
import "./conversation.css";

// The connect act's dialog shell, with the two shapes its callers give it.
//
// What the shell owns is the ASK — the provider's mark, the headline, and the
// plain-words line saying what connecting reads — before the reader leaves for
// the provider's own consent screen or hands over a credential. The body below
// it belongs to the caller, and the two frames here are the two bodies that
// exist: a mailbox, whose disclosure is the intro line, and LinkedIn, whose
// own panel carries the disclosure and therefore takes no intro. Never both,
// which would say the same thing twice.

const meta: Meta<typeof ConnectDialog> = {
  title: "Onboarding/Conversation/Connect dialog",
  component: ConnectDialog,
  // Modal portals to document.body, so `#storybook-root` holds the preview
  // decorator and nothing else. The play is what actually proves the dialog
  // mounted; without it the render gate would bless an empty frame.
  play: async () => {
    const dialog = within(await screen.findByRole("dialog"));
    await dialog.findByRole("heading");
  },
};
export default meta;

type Story = StoryObj<typeof ConnectDialog>;

/**
 * The mailbox ask, as the connect act opens it: the provider's mark, what it
 * brings, and the scope line the reader is about to approve elsewhere.
 */
function MailboxAsk() {
  const t = useT();
  return (
    <ConnectDialog
      open
      onClose={() => {}}
      providerMarkKey="google"
      headline={t("ob.conv.connect.dialogHeadlineAccess", { name: "Google" })}
      intro={t("ob.conv.connect.dialogIntro", {
        brings: t("ob.conv.connect.gmailBrings"),
      })}
    >
      <p>{t("ob.conv.connect.scopeGoogle")}</p>
      <Button onClick={() => {}}>{t("ob.conv.connect.connectCta")}</Button>
    </ConnectDialog>
  );
}

/**
 * LinkedIn's ask. No intro line: the panel's own field states what is being
 * saved, and a second sentence above it would be the same disclosure twice.
 */
function LinkedinAsk() {
  const t = useT();
  return (
    <ConnectDialog
      open
      onClose={() => {}}
      providerMarkKey="linkedin"
      headline={t("ob.conv.linkedin.dialogHeadline")}
    >
      <Field label={t("ob.conv.linkedin.profileLabel")}>
        {(control) => (
          <TextInput
            {...control}
            placeholder={t("ob.conv.linkedin.profilePlaceholder")}
          />
        )}
      </Field>
      <Button onClick={() => {}}>{t("ob.conv.connect.saveCta")}</Button>
    </ConnectDialog>
  );
}

/** A mailbox, which is the gate the act waits on. */
export const AMailboxAsk: Story = {
  render: () => (
    <StoryProviders>
      <MailboxAsk />
    </StoryProviders>
  ),
};

/** LinkedIn, offered beside it and never holding the act up. */
export const TheLinkedinAsk: Story = {
  render: () => (
    <StoryProviders>
      <LinkedinAsk />
    </StoryProviders>
  ),
};
