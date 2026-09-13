// Why a project's write verbs are unavailable, as one sentence.
//
// Its own file because project360.tsx sits at a frozen line ceiling. Every
// write affordance on that page asks this ONE question, asked once: an
// archived project takes no changes, and one this caller cannot write takes
// none from them. One answer and one rendered sentence, so the page cannot
// disagree with itself about whether the record is writable.

import type { components } from "../api/schema";
import { useRecordWriteRefusal } from "../app/capability";
import { useT } from "../i18n";

type Project = components["schemas"]["Project"];

export function useProjectVerbRefusal(project: Project): string | undefined {
  const t = useT();
  return useRecordWriteRefusal("project", project, {
    archived: t("project.archivedReadOnly"),
    notYours: t("project.notYoursToChange"),
  });
}
