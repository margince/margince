// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Callout } from "../design-system/callout";
import { useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf } from "./common";

// What a provider card says when one of its switches did not take.
//
// Its own file because both bands are the same shape and both had drawn it by
// hand: the notice belongs UNDER the row whose flip failed, at the row's full
// width, and never squeezed into the control column — a sentence sharing a
// nowrap flex line with a switch is a sentence nobody reads.

/**
 * A refused switch, under the row it belongs to: the claim as the heading, the
 * server's own cause under it, wrapped in the plate that gives a non-row child
 * of the list its interval and drops the hairline.
 *
 * Not exported: the two notices below are what a card asks for, because each
 * knows which of its writes this is about and no caller should have to pick a
 * heading for it.
 */
function RowRefusal({
  titleKey,
  error,
}: Readonly<{ titleKey: MessageKey; error: unknown }>) {
  const t = useT();
  return (
    <div className="provider-row-note">
      <Callout tone="danger" kind="outcome" title={t(titleKey)}>
        {problemMessageOf(error, t)}
      </Callout>
    </div>
  );
}

/**
 * The automatic-lookup switch, and the READ behind it as well as the write.
 *
 * A posture we could not ask for renders the switch off, and "off" is a claim
 * about the installation — so a failed GET has to say so rather than let the
 * control answer a question nobody could reach. The heading says WHICH of the
 * two happened, because "something went wrong" leaves the reader unable to tell
 * whether their press landed.
 */
export function PostureRefused({
  writeError,
  readError,
}: Readonly<{ writeError: unknown; readError: unknown }>) {
  if (!writeError && !readError) {
    return null;
  }
  return (
    <RowRefusal
      titleKey={
        writeError
          ? "provider.postureWriteFailed"
          : "provider.postureReadFailed"
      }
      error={writeError ?? readError}
    />
  );
}

/** One priced category's buy switch, refused. */
export function BuyableRefused({ error }: Readonly<{ error: unknown }>) {
  if (!error) {
    return null;
  }
  return <RowRefusal titleKey="provider.buyableWriteFailed" error={error} />;
}
