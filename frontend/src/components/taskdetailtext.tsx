// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../api/schema";
import { GeneratedText } from "../design-system/generatedtext";
import { NoteMarkdown } from "../design-system/markdown";
import { ReaderRendering } from "../design-system/readerrendering";
import { useLocale, useT } from "../i18n";
import { useMe } from "../lib/session";
import { displayedBody, displayedSubject } from "../lib/systemdisplay";

type Activity = components["schemas"]["Activity"];

export function TaskDetailTitle({
  task,
}: Readonly<{ task: Activity | undefined }>) {
  const t = useT();
  if (!task) return t("tasks.detail");
  return (
    <GeneratedText activityId={task.id} display={task.system_display} inline>
      {displayedSubject(task) ?? t("tasks.detail")}
    </GeneratedText>
  );
}

export function TaskDetailReading({ task }: Readonly<{ task: Activity }>) {
  const { locale } = useLocale();
  const t = useT();
  const session = useMe();
  const display = task.system_display;
  const automatic = Boolean(display?.generated_parts?.length);
  const noModel =
    automatic &&
    session.data &&
    session.data.reader_rendering_available !== true;
  return (
    <>
      {noModel && <p className="t-caption">{t("readerRendering.noModel")}</p>}
      <ReaderRendering
        key={`${task.id}:${task.version}:${locale}`}
        activityId={task.id}
        sourceVersion={task.version ?? undefined}
        language={automatic || display?.body ? locale : task.language}
        note
      >
        {task.body && (
          <div className="t-body task-detail-body">
            <GeneratedText
              activityId={task.id}
              display={display}
              part="body"
              renderText={(text) => <NoteMarkdown source={text} />}
            >
              <NoteMarkdown source={displayedBody(task) ?? task.body} />
            </GeneratedText>
          </div>
        )}
      </ReaderRendering>
    </>
  );
}
