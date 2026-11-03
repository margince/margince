// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { RailEdit } from "../../components/railfield";
import { saveRecordEdit } from "../../components/recordedit";
import { rawRecord } from "../../components/recordfieldtypes";
import { useOwnerPicker } from "../../components/recordowners";
import { useT } from "../../i18n";
import { useProjectVerbRefusal } from "./project360.refusal";
import {
  mapProjectUpdate,
  type Project,
  projectEditRecord,
  projectFields,
} from "./projects.form";

/**
 * How a project's fields change from its left column: the edit dialog's
 * fields and mapping, saved as only what the reader changed.
 */
export function useProjectEdit(project: Project): RailEdit {
  const t = useT();
  const owner = useOwnerPicker(project.owner_id);
  const refusal = useProjectVerbRefusal(project);
  return {
    kind: "project",
    canEdit: !refusal,
    fields: projectFields(t, { owner, mode: "edit" }),
    record: { ...projectEditRecord(project), original: project },
    save: async (values, _rows, opened) =>
      saveRecordEdit(
        "project",
        rawRecord(opened),
        mapProjectUpdate({ ...opened, ...values }),
      ),
  };
}
