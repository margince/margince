// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What the bulk dialog calls each record type and verb, and how it draws a
// record's state before and after. Kept apart from the dialog so a new verb is
// taught its words here, in one place.

import type { components } from "../api/schema";
import { Badge } from "../design-system/atoms";
import { type PluralBase, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import type { BulkChangeRequest, Translate } from "./bulkchange";
import { OwnerName } from "./entityref";

type BulkRecordType = components["schemas"]["BulkRecordType"];
type BulkVerb = components["schemas"]["BulkVerb"];
type BulkSampleRow = components["schemas"]["BulkSampleRow"];
type BulkSkip = components["schemas"]["BulkSkip"];
type BulkSkipReason = components["schemas"]["BulkSkipReason"];

export type RecordKind = Readonly<{
  list: string;
  record: string;
  unit: MessageKey;
  done: PluralBase;
  undone: PluralBase;
}>;

export const RECORD_KINDS: Readonly<Record<BulkRecordType, RecordKind>> = {
  contact: {
    list: "contacts",
    record: "contact",
    unit: "unit.contacts",
    done: "bulk.doneContacts",
    undone: "bulk.undoneContacts",
  },
  company: {
    list: "companies",
    record: "company",
    unit: "unit.companies",
    done: "bulk.doneCompanies",
    undone: "bulk.undoneCompanies",
  },
  deal: {
    list: "deals",
    record: "deal",
    unit: "unit.deals",
    done: "bulk.doneDeals",
    undone: "bulk.undoneDeals",
  },
  lead: {
    list: "leads",
    record: "lead",
    unit: "unit.leads",
    done: "bulk.doneLeads",
    undone: "bulk.undoneLeads",
  },
  // A task or a promise on the Worklist. The task's own reads sit under
  // ["activity", id], and the Worklist under its own key.
  worklist_item: {
    list: "worklist",
    record: "activity",
    unit: "unit.worklistItems",
    done: "bulk.doneWorklistItems",
    undone: "bulk.undoneWorklistItems",
  },
};

export function isListVerb(verb: BulkVerb): boolean {
  return verb === "add_to_list" || verb === "remove_from_list";
}

export function isTagVerb(verb: BulkVerb): boolean {
  return verb === "add_tag" || verb === "remove_tag";
}

const SKIP_REASONS: Readonly<Record<BulkSkipReason, MessageKey>> = {
  not_found: "bulk.reason.not_found",
  not_writable: "bulk.reason.not_writable",
  changed_since_preview: "bulk.reason.changed_since_preview",
  no_change: "bulk.reason.no_change",
  anchor_company: "bulk.reason.anchor_company",
  not_previewed: "bulk.reason.not_previewed",
  refused: "bulk.reason.refused",
  changed_since_batch: "bulk.reason.changed_since_batch",
  merged: "bulk.reason.merged",
  erased: "bulk.reason.erased",
  value_taken: "bulk.reason.value_taken",
  no_previous_owner: "bulk.reason.no_previous_owner",
};

// `no_change` means what the verb could not change: the owner, the Shortlist
// membership, the tag or the done state was already as asked.
function noChangeReason(verb: BulkVerb): MessageKey {
  if (verb === "complete") {
    return "bulk.reason.no_change_done";
  }
  if (isListVerb(verb)) {
    return "bulk.reason.no_change_list";
  }
  if (isTagVerb(verb)) {
    return "bulk.reason.no_change_tag";
  }
  return "bulk.reason.no_change";
}

// The single-record rules a `refused` skip names by code. A code missing here
// falls back to the server's English `message`.
const REFUSAL_CODES: Readonly<Record<string, MessageKey>> = {
  sole_project_company: "bulk.refusal.sole_project_company",
  locked: "bulk.refusal.locked",
  anchor_protected: "bulk.refusal.anchor_protected",
  required: "bulk.refusal.required",
};

export function SkipReason({
  skip,
  verb,
}: Readonly<{ skip: BulkSkip; verb: BulkVerb }>) {
  const t = useT();
  if (skip.reason === "no_change") {
    return <span>{t(noChangeReason(verb))}</span>;
  }
  if (skip.reason !== "refused") {
    return <span>{t(SKIP_REASONS[skip.reason])}</span>;
  }
  const known =
    skip.code && Object.hasOwn(REFUSAL_CODES, skip.code)
      ? REFUSAL_CODES[skip.code]
      : undefined;
  if (known) {
    return <span>{t(known)}</span>;
  }
  return <span>{skip.message ?? t(SKIP_REASONS.refused)}</span>;
}

export function SampleState({
  verb,
  state,
}: Readonly<{ verb: BulkVerb; state: BulkSampleRow["before"] }>) {
  const t = useT();
  if (verb === "reassign_owner") {
    return <OwnerName ownerId={state.owner_id} unowned={t("list.unowned")} />;
  }
  if (isListVerb(verb)) {
    return (
      <span>
        {state.listed ? t("bulk.stateListed") : t("bulk.stateNotListed")}
      </span>
    );
  }
  if (isTagVerb(verb)) {
    return (
      <span>
        {state.tagged ? t("bulk.stateTagged") : t("bulk.stateNotTagged")}
      </span>
    );
  }
  if (verb === "create_task") {
    return (
      <span>
        {state.task_id ? t("bulk.stateNewTask") : t("bulk.stateNoTask")}
      </span>
    );
  }
  if (verb === "complete") {
    return (
      <span>{state.done ? t("bulk.stateDone") : t("bulk.stateOpen")}</span>
    );
  }
  return state.archived ? (
    <Badge tone="warning">{t("record.archived")}</Badge>
  ) : (
    <span>{t("bulk.stateActive")}</span>
  );
}

type DialogWords = Readonly<{
  title: string;
  confirm: string;
  danger: boolean;
}>;

// The words of a verb that writes beside the record — a Shortlist, a tag, a
// task — or marks a Worklist item done.
function besideWords(
  request: BulkChangeRequest,
  unit: string,
  t: Translate,
): DialogWords | undefined {
  const list = request.list?.name ?? "";
  const tag = request.tag?.name ?? "";
  switch (request.verb) {
    case "add_to_list":
      return {
        title: t("bulk.titleAddToList", { unit, list }),
        confirm: t("bulk.confirmAddToList"),
        danger: false,
      };
    case "remove_from_list":
      return {
        title: t("bulk.titleRemoveFromList", { unit, list }),
        confirm: t("bulk.confirmRemoveFromList"),
        danger: true,
      };
    case "add_tag":
      return {
        title: t("bulk.titleAddTag", { unit, tag }),
        confirm: t("bulk.confirmAddTag"),
        danger: false,
      };
    case "remove_tag":
      return {
        title: t("bulk.titleRemoveTag", { unit, tag }),
        confirm: t("bulk.confirmRemoveTag"),
        danger: true,
      };
    case "create_task":
      return {
        title: t("bulk.titleCreateTask", { unit }),
        confirm: t("bulk.confirmCreateTask"),
        danger: false,
      };
    case "complete":
      return {
        title: t("bulk.titleComplete", { unit }),
        confirm: t("bulk.confirmComplete"),
        danger: false,
      };
    default:
      return undefined;
  }
}

/** The dialog's heading and confirm verb for one press. */
export function dialogWords(
  request: BulkChangeRequest,
  t: Translate,
): DialogWords {
  const unit = t(RECORD_KINDS[request.recordType].unit);
  if (request.undoOf !== undefined) {
    return {
      title: t("bulk.titleUndo", { unit }),
      confirm: t("bulk.confirmUndo"),
      danger: false,
    };
  }
  if (request.verb === "archive") {
    return {
      title: t("bulk.titleArchive", { unit }),
      confirm: t("bulk.confirmArchive", { unit }),
      danger: true,
    };
  }
  return (
    besideWords(request, unit, t) ?? {
      title: t("bulk.titleReassign", { unit }),
      confirm: t("bulk.confirmReassign"),
      danger: false,
    }
  );
}
