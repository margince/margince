// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useEffect, useRef } from "react";
import { useT } from "../i18n";
import { problemMessageOf } from "../screens/common";
import { usePrefersReducedMotion } from "./motion";

type ErrorLineProps = Readonly<
  (
    | { error: unknown; children?: never }
    | { error?: never; children: ReactNode }
  ) & {
    id?: string;
    actions?: ReactNode;
    inline?: true;
    standing?: true;
  }
>;

/**
 * The one line that says a write or a read under a control failed, in the
 * danger ink and announced the moment it arrives. `error` takes a thrown value
 * and draws nothing while it is null, so a caller hands it a query's `error`
 * straight; `children` takes a sentence the caller already translated.
 * `inline` draws a `<span>`, for a refusal that sits in its control's row.
 * `standing` drops the alert: a state true when the surface drew is not news.
 * The parent spaces it: the line owns no margin. Inside a dialog an alerting
 * line scrolls itself into view, since a pinned footer's save fails off-screen.
 */
export function ErrorLine({
  error,
  children,
  id,
  actions,
  inline,
  standing,
}: ErrorLineProps) {
  const t = useT();
  // A `false` error is a guard's short-circuit (`isError && error`), not a
  // failure, and an empty string says nothing an alert could announce.
  const absent =
    error === undefined || error === null || error === false || error === "";
  const line = useScrolledIntoDialogView(
    standing === true,
    absent ? null : error,
  );
  const message = absent ? children : problemMessageOf(error, t);
  if (
    message === undefined ||
    message === null ||
    message === false ||
    message === ""
  ) {
    return null;
  }
  const Line = inline ? "span" : "p";
  return (
    <Line
      ref={line}
      role={standing ? undefined : "alert"}
      id={id}
      className={actions ? "t-danger error-line" : "t-danger"}
    >
      {message}
      {actions}
    </Line>
  );
}

// Keyed on the thrown value, new per failed attempt, so an identical refusal
// still scrolls; `children` is a new element every render, so it keys on text.
function useScrolledIntoDialogView(standing: boolean, failure: unknown) {
  const line = useRef<HTMLParagraphElement & HTMLSpanElement>(null);
  const shown = useRef<unknown>(null);
  const reduced = usePrefersReducedMotion();
  useEffect(() => {
    const said = failure ?? line.current?.textContent ?? null;
    if (said === shown.current) {
      return;
    }
    shown.current = said;
    if (!standing && line.current?.closest(".modal")) {
      // jsdom has no scrollIntoView; the browser always does.
      line.current.scrollIntoView?.({
        block: "nearest",
        behavior: reduced ? "auto" : "smooth",
      });
    }
  });
  return line;
}
