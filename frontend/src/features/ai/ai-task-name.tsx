// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Popover } from "../../design-system/popover";
import { useT } from "../../i18n";

/**
 * A task's name, opening what the task does and the key it runs under. Every
 * screen that names an AI task names it this way. The key a log uses is then
 * one press away, and never stands in for the name.
 */
export function TaskName({
  name,
  task,
  mode,
  summary,
  details,
  inTitle = false,
}: Readonly<{
  name: string;
  // The task's key, as the contract and the calls log spell it.
  task: string;
  // How the task runs, already in words, beside the key.
  mode?: string;
  summary: string | undefined;
  // What else the reader may want behind the name, below what the task does.
  details?: ReactNode;
  // A dialog's title is named by its content. There the trigger says the name
  // alone, so the dialog reads the task's name rather than "what it does".
  inTitle?: boolean;
}>) {
  const t = useT();
  if (!summary && details == null && task === name) {
    return <span>{name}</span>;
  }
  return (
    <Popover
      className="evmark-trigger"
      label={
        inTitle ? (
          name
        ) : (
          <>
            <span aria-hidden>{name}</span>
            <span className="sr-only">
              {t("aiTasks.whatItDoes", { task: name })}
            </span>
          </>
        )
      }
    >
      {summary ? <p>{summary}</p> : null}
      <p className="t-caption">
        <code>{task}</code>
        {mode ? ` · ${mode}` : null}
      </p>
      {details}
    </Popover>
  );
}
