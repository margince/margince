// Why a project's write verbs are unavailable, as one sentence.
//
// Every write affordance on project360.tsx asks this one question, once. An
// archived project takes no changes, and one this caller cannot write takes
// none from them. One answer and one rendered sentence, so the page cannot
// disagree with itself about whether the record is writable.

import type { components } from "../../api/schema";
import { useRecordWriteRefusal } from "../../lib/recordrefusal";

type Project = components["schemas"]["Project"];

export function useProjectVerbRefusal(project: Project): string | undefined {
  return useRecordWriteRefusal("project", project);
}
