// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// "Add to Shortlist" over a selection: pick one of the Shortlists this reader
// looks after, and the bulk dialog previews and confirms the addition like any
// other bulk change. Absent while lists are switched off.

import { Select } from "../design-system/select";
import { useT } from "../i18n";
import {
  type ListRecordType,
  useLists,
  useListsAvailable,
} from "./lists.queries";

export function ShortlistVerb({
  recordType,
  disabled,
  onPick,
}: Readonly<{
  recordType: ListRecordType;
  disabled: boolean;
  onPick: (list: Readonly<{ id: string; name: string }>) => void;
}>) {
  const t = useT();
  const available = useListsAvailable();
  const shortlists = useLists(
    { entityType: recordType, listType: "static" },
    available,
  );
  const editable = (shortlists.data?.data ?? []).filter(
    (list) => list.can_edit && !list.archived_at,
  );
  if (!available || editable.length === 0) {
    return null;
  }
  return (
    <Select
      aria-label={t("bulk.addToShortlist")}
      value=""
      placeholder={t("bulk.addToShortlist")}
      disabled={disabled}
      onChange={(id) => {
        const list = editable.find((candidate) => candidate.id === id);
        if (list) {
          onPick({ id: list.id, name: list.name });
        }
      }}
      options={editable.map((list) => ({ value: list.id, label: list.name }))}
    />
  );
}
