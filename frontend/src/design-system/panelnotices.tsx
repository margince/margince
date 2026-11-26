// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode, Ref } from "react";
import { useT } from "../i18n";
import { problemMessageOf } from "../lib/problem";
import { Callout } from "./callout";
import { useScrolledIntoDialogView } from "./errorline";
import { PanelBody } from "./panel";
import "./panel.css";

// Said once for the whole card rather than on each refused control.
export function PanelNotices({
  readOnly,
  refused,
}: Readonly<{
  /** The read-only posture's title, or false for a reader who may write. */
  readOnly?: string | false;
  /** The card's last refused write: its title, the failure behind it, and an
   * id the refused controls name in `aria-describedby`. */
  refused?: Readonly<{ title: string; error: unknown; id?: string }>;
}>) {
  const t = useT();
  if (!readOnly && refused === undefined) return null;
  return (
    <PanelBody className="panel-notices">
      {readOnly && <Callout kind="standing" title={readOnly} />}
      {refused !== undefined && (
        <RefusedCallout title={refused.title} id={refused.id}>
          {problemMessageOf(refused.error, t)}
        </RefusedCallout>
      )}
    </PanelBody>
  );
}

const absent = (said: unknown) =>
  said === undefined || said === null || said === false || said === "";

/**
 * The refused write of a dialog or a drawer, in the shape a card gives its own.
 * It draws nothing while `error` and `message` are both absent, so a caller
 * hands it the mutation's failure straight. It scrolls itself into view, since
 * the dialog's pinned foot can hide a notice that lands under it.
 */
export function DialogRefusal({
  title,
  error,
  message,
  actions,
  id,
}: Readonly<{
  title: string;
  /** The thrown failure; its words come from `problemMessageOf`. */
  error?: unknown;
  /** The caller's own words for the failure, in place of the server's. */
  message?: ReactNode;
  /** What the reader can do about it: a retry, a way back out. */
  actions?: ReactNode;
  /** For the `aria-describedby` of the refused control. */
  id?: string;
}>) {
  const t = useT();
  const notice = useScrolledIntoDialogView<HTMLDivElement>(
    false,
    absent(error) ? null : error,
  );
  if (absent(error) && absent(message)) return null;
  return (
    <RefusedCallout title={title} id={id} actions={actions} ref={notice}>
      {absent(message) ? problemMessageOf(error, t) : message}
    </RefusedCallout>
  );
}

// The one spelling of a refusal for a card and a dialog alike.
function RefusedCallout({
  title,
  id,
  actions,
  ref,
  children,
}: Readonly<{
  title: string;
  id?: string;
  actions?: ReactNode;
  ref?: Ref<HTMLDivElement>;
  children: ReactNode;
}>) {
  return (
    <Callout
      kind="outcome"
      tone="danger"
      title={title}
      id={id}
      actions={actions}
      ref={ref}
    >
      {children}
    </Callout>
  );
}
