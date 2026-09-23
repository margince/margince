// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { useT } from "../i18n";
import { problemMessageOf } from "../screens/common";

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
 * The parent spaces it: the line owns no margin.
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
  // A `false` error is a guard's short-circuit (`isError && error`), not a failure.
  const absent = error === undefined || error === null || error === false;
  const message = absent ? children : problemMessageOf(error, t);
  if (message === undefined || message === null || message === false) {
    return null;
  }
  const Line = inline ? "span" : "p";
  return (
    <Line
      role={standing ? undefined : "alert"}
      id={id}
      className={actions ? "t-danger error-line" : "t-danger"}
    >
      {message}
      {actions}
    </Line>
  );
}
