// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The record's write-email verb and the composer it opens.

import { Mail } from "lucide-react";
import { type ReactNode, useState } from "react";
import { IconAction } from "../../design-system/iconaction";
import { useT } from "../../i18n";
import { ComposeModal, type RelinkKind } from "../mail/compose";

/**
 * EmailVerb is the ONE way a record header offers writing. Every record page
 * draws it, so a reader moving from a company to the contact on it finds the
 * same control in the same place. A page whose composer can open on another
 * transport hands in the label and glyph it resolved; the default is mail.
 *
 * It is SQUARE, because an envelope is one of the handful of verbs a reader
 * already knows from the glyph — the same reason Call and Meetings are squares
 * beside it. The header's remaining verbs either keep their words outside the
 * menu or move into it; a row where Write alone wore a label would put the
 * page's most frequent action at the weight of its rarest.
 *
 * `label` is therefore the NAME rather than visible text: `IconAction` speaks
 * it through `aria-label` and shows it on hover, so a transport-specific verb
 * ("Send a WhatsApp") still reaches both readers. The refusals pass straight
 * through — a header with nothing to write to says why on hover and under the
 * control, rather than offering a square that does nothing.
 */
export function EmailVerb({
  label,
  icon,
  onClick,
  disabled,
  reason,
  reasonId,
  reasonTip,
  word,
  keyShortcuts,
}: Readonly<{
  /** The verb, translated — spoken as the name and shown as the tip. */
  label?: string;
  /** The tile word in the verb bar; "Email" while `label` is the default. */
  word?: string;
  icon?: ReactNode;
  onClick: () => void;
  disabled?: boolean;
  reason?: string;
  reasonId?: string;
  reasonTip?: string;
  keyShortcuts?: string;
}>) {
  const t = useT();
  return (
    <IconAction
      label={label ?? t("contact.action.email")}
      word={word ?? (label === undefined ? t("record.verb.email") : undefined)}
      icon={icon ?? <Mail aria-hidden="true" />}
      disabled={disabled}
      reason={reason}
      reasonId={reasonId}
      reasonTip={reasonTip}
      keyShortcuts={keyShortcuts}
      onClick={onClick}
    />
  );
}

/**
 * Whether the composer's open state is the CALLER's, for a page with a second
 * control that opens this same composer: a lead's "Answer" row hands its Reply
 * verb the header's own modal rather than mounting a copy of it.
 *
 * Both halves or neither, spelled as a union rather than as two optional
 * props. A caller that passed only the setter never saw the composer open,
 * because the state it drove was not the state this component reads; one that
 * passed only the value could never ask for it to close. Both were a type the
 * compiler accepted and a control the reader could not work.
 */
type Lifted =
  | Readonly<{ open: boolean; onOpenChange: (open: boolean) => void }>
  | Readonly<{ open?: undefined; onOpenChange?: undefined }>;

/**
 * RecordEmailVerb is the header's Email verb with its own composer, for a
 * record whose page keeps no composer state of its own: the deal and the lead.
 */
export function RecordEmailVerb({
  entityType,
  entityId,
  contactId,
  recordAddress,
  disabledReasonId,
  reasonTip,
  disabled,
  keyShortcuts,
  open,
  onOpenChange,
}: Readonly<{
  entityType: RelinkKind;
  entityId: string;
  contactId?: string;
  /** The record's own address, for a first message to it. See ComposeModal. */
  recordAddress?: string;
  disabledReasonId?: string;
  /** The sentence `disabledReasonId` names, for the verb's tooltip. */
  reasonTip?: string;
  disabled?: boolean;
  keyShortcuts?: string;
}> &
  Lifted) {
  const [ownComposing, setOwnComposing] = useState(false);
  const composing = open ?? ownComposing;
  const setComposing = onOpenChange ?? setOwnComposing;
  // Whether the composer has ever been asked for, open state included: a
  // caller that opens it from elsewhere on the page still has to mount it
  // here, the one place this component's own modal lives.
  const [everComposed, setEverComposed] = useState(composing);
  if (composing && !everComposed) {
    setEverComposed(true);
  }
  return (
    <>
      <EmailVerb
        disabled={disabled}
        reasonId={disabledReasonId}
        reasonTip={reasonTip}
        keyShortcuts={keyShortcuts}
        onClick={() => setComposing(true)}
      />
      {/* Not drawn until the verb has been pressed once, and mounted from
          then on. A composer mounted with the record would read on every
          render of a page nobody is writing from; one unmounted the moment it
          closes has no frame left to animate out on. */}
      {everComposed && (
        // Keyed by the record, so navigating to another one while the
        // composer is open remounts it rather than re-pointing it.
        <ComposeModal
          key={entityId}
          entityType={entityType}
          entityId={entityId}
          contactId={contactId}
          recordAddress={recordAddress}
          kind="email"
          open={composing}
          onClose={() => setComposing(false)}
        />
      )}
    </>
  );
}
