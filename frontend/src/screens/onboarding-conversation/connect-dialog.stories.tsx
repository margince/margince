import type { Meta, StoryObj } from "@storybook/react-vite";
import { type ReactNode, useState } from "react";
import { Button } from "../../design-system/atoms";
import { StoryProviders } from "../story-utils";
import { ConnectDialog } from "./connect-dialog";
import "./conversation.css";

// The connect act's one dialog shell, drawn on its own.
//
// Every story opens it ON MOUNT, because a dialog rendered closed screenshots
// as an empty canvas — and the trigger stays behind it, both so the reader can
// watch the surface arrive again after dismissing it and because the dialog
// portals out of the story root, which would otherwise be empty.
//
// What separates the asks is who carries the DISCLOSURE: a mailbox provider
// needs `intro` to say in plain words what connecting reads, and LinkedIn's own
// scope list already says it, so passing both would say the same thing twice.
// The German one is here because the headline wraps where the English does not.
//
// There is no closed story. The way out is `Modal`'s own, which this shell no
// longer draws for itself, and a story of a dialog that is not there documents
// nothing.

const meta: Meta<typeof ConnectDialog> = {
  title: "Onboarding/Connect dialog",
  component: ConnectDialog,
  parameters: { layout: "padded" },
};
export default meta;

type Story = StoryObj<typeof ConnectDialog>;

function Ask({
  providerMarkKey,
  headline,
  intro,
  children,
  locale,
}: Readonly<{
  providerMarkKey: string;
  headline: string;
  intro?: string;
  children: ReactNode;
  locale?: "de";
}>) {
  const [open, setOpen] = useState(true);
  return (
    <StoryProviders locale={locale}>
      <Button variant="primary" onClick={() => setOpen(true)}>
        Connect
      </Button>
      <ConnectDialog
        open={open}
        onClose={() => setOpen(false)}
        providerMarkKey={providerMarkKey}
        headline={headline}
        intro={intro}
      >
        {children}
      </ConnectDialog>
    </StoryProviders>
  );
}

/** A mailbox provider's ask: the mark, the headline, and the plain-words
 *  disclosure of what connecting reads before the reader leaves for Google's
 *  own consent screen. */
export const MailboxAsk: Story = {
  render: () => (
    <Ask
      providerMarkKey="google"
      headline="Connect your Google mailbox"
      intro="Margince reads the mail you send and receive so it can file it against the right account. It never sends on your behalf without you pressing Send."
    >
      <div className="ob-connect-dialog-actions">
        <Button variant="primary">Continue to Google</Button>
        <button type="button" className="ob-connect-dialog-notnow">
          Not now
        </button>
      </div>
    </Ask>
  ),
};

/** LinkedIn's ask carries its own scope list, so the shell passes no `intro` —
 *  the disclosure is in the content rather than above it. */
export const ScopesInTheContent: Story = {
  render: () => (
    <Ask providerMarkKey="linkedin" headline="Connect LinkedIn">
      <div className="ob-connect-linkedin-panel">
        <p className="t-body">Margince would read, from your profile:</p>
        <ul className="t-body">
          <li>your name and headline</li>
          <li>the companies you follow</li>
          <li>connections you share with a contact</li>
        </ul>
      </div>
      <div className="ob-connect-dialog-actions">
        <Button variant="primary">Continue to LinkedIn</Button>
        <button type="button" className="ob-connect-dialog-notnow">
          Not now
        </button>
      </div>
    </Ask>
  ),
};

/** A credential ask rather than a redirect: IMAP hands over a password in the
 *  dialog itself, so the body is a form and the headline says whose. */
export const CredentialAsk: Story = {
  render: () => (
    <Ask
      providerMarkKey="imap"
      headline="Connect a mailbox over IMAP"
      intro="Margince signs in as you and reads your mail. The password is stored encrypted and is never shown again."
    >
      <div className="ob-connect-dialog-actions">
        <Button variant="primary">Sign in</Button>
        <button type="button" className="ob-connect-dialog-notnow">
          Not now
        </button>
      </div>
    </Ask>
  ),
};

/** The same mailbox ask in German: the longer copy, and a headline that wraps
 *  where the English one does not. */
export const MailboxAskGerman: Story = {
  render: () => (
    <Ask
      locale="de"
      providerMarkKey="graph"
      headline="Microsoft-Postfach verbinden"
      intro="Margince liest Ihre gesendeten und empfangenen Nachrichten, um sie dem richtigen Konto zuzuordnen. Ohne Ihren Klick auf Senden wird nichts verschickt."
    >
      <div className="ob-connect-dialog-actions">
        <Button variant="primary">Weiter zu Microsoft</Button>
        <button type="button" className="ob-connect-dialog-notnow">
          Nicht jetzt
        </button>
      </div>
    </Ask>
  ),
};
