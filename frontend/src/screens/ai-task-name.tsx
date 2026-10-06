// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import { Popover } from "../design-system/popover";
import { useT } from "../i18n";

/**
 * A task's name, opening what the task does. The AI tasks table and the usage
 * table both name tasks to a reader who has to judge them — which model should
 * serve one, what one cost — and the name alone rarely says enough.
 */
export function TaskName({
  name,
  summary,
  details,
}: Readonly<{
  name: string;
  summary: string | undefined;
  // What else the reader may want behind the name, below what the task does.
  details?: ReactNode;
}>) {
  const t = useT();
  if (!summary && !details) return <span>{name}</span>;
  return (
    <Popover
      className="evmark-trigger"
      label={
        <>
          <span aria-hidden>{name}</span>
          <span className="sr-only">
            {t("aiTasks.whatItDoes", { task: name })}
          </span>
        </>
      }
    >
      {summary ? <p>{summary}</p> : null}
      {details}
    </Popover>
  );
}
