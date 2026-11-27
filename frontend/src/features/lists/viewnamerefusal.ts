// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { nameRefusal } from "../../design-system/namedialog";
import type { MessageKey } from "../../i18n/en";

export function viewNameRefusal(
  error: unknown,
  t: (key: MessageKey) => string,
) {
  return nameRefusal(error, t, {
    label: t("views.name"),
    duplicate: VIEW_NAME_TAKEN_COPY,
    code: VIEW_NAME_TAKEN_CODE,
  });
}

export function viewNameConflict(t: (key: MessageKey) => string) {
  return {
    field: "name",
    sentence: t(VIEW_NAME_TAKEN_COPY),
    code: VIEW_NAME_TAKEN_CODE,
  };
}
const VIEW_NAME_TAKEN_CODE = "view_name_taken";
const VIEW_NAME_TAKEN_COPY = "views.nameTaken";
