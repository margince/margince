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
  }
>;

/**
 * The one line that says a write or a read under a control failed, in the
 * danger ink and announced the moment it arrives. It replaces four spellings
 * of the same `<p>`, half of which lacked either the ink or the announcement.
 * `error` takes a thrown value and draws nothing while it is null, so a caller
 * hands it a query's `error` straight; `children` takes a sentence the caller
 * already translated. The parent spaces it: the line owns no margin.
 */
export function ErrorLine({ error, children, id, actions }: ErrorLineProps) {
  const t = useT();
  const message =
    error === undefined || error === null
      ? children
      : problemMessageOf(error, t);
  if (message === undefined || message === null || message === false) {
    return null;
  }
  return (
    <p
      role="alert"
      id={id}
      className={actions ? "t-danger error-line" : "t-danger"}
    >
      {message}
      {actions}
    </p>
  );
}
