// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { MessageKey } from "../../i18n/en";

export const ACTION_WORDS: Readonly<Record<string, MessageKey>> = {
  create_task: "auto.action.createTask",
  notify: "auto.action.notify",
  assign_owner: "auto.action.assignOwner",
  set_field: "auto.action.setField",
  draft_email: "auto.action.draftEmail",
  request_approval: "auto.action.requestApproval",
  add_to_shortlist: "auto.action.addToShortlist",
};
