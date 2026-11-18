import { Callout } from "../../design-system/callout";
import { useT } from "../../i18n";
import { ImapMailboxForm } from "../capture/imap-connect-form";

/** A leg that did not land. A refusal the reader gave is theirs to answer
 *  again (amber); anything else broke (red). */
export function ReturnWarn({
  title,
  body,
  tone = "danger",
}: Readonly<{ title: string; body: string; tone?: "warning" | "danger" }>) {
  return (
    <Callout tone={tone} kind="outcome" title={title}>
      <p>{body}</p>
    </Callout>
  );
}

// IMAP: the mailbox form Settings shows (imap-connect-form.tsx), with the
// step's own dismissal and action row, so the two cannot drift apart.
//
// `onDismiss` puts the form away without connecting: the reader changed their
// mind about this provider, not about the step, which the surface skips.
export function ImapConnectPanel({
  onDone,
  onDismiss,
  onPendingChange,
}: Readonly<{
  /** Closes the panel once the mailbox is live; the step is left from the
   * connect surface, never from here (see `OAuthReturnPanel`). */
  onDone: () => void;
  onDismiss: () => void;
  /** Told whenever the connect POST's in-flight state changes, so the caller
   * can hold its own ways off this form (another vendor, the skip) too. */
  onPendingChange?: (pending: boolean) => void;
}>) {
  const t = useT();
  return (
    <ImapMailboxForm
      dismissLabel={t("ob.s4.notNow")}
      onDismiss={onDismiss}
      onConnected={onDone}
      onPendingChange={onPendingChange}
      renderActions={(actions) => (
        <div className="ob-connect-actions">{actions}</div>
      )}
    />
  );
}
