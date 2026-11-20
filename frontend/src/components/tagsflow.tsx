// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId, useState } from "react";
import { DrawerBody, DrawerHead } from "../design-system/drawerbands";
import { Heading } from "../design-system/heading";
import { Modal } from "../design-system/modal";
import type { DetailRow } from "../design-system/recorddetails";
import { useT } from "../i18n";
import { useCanWriteRecord, type WritableRecord } from "../lib/capability";
import { type TaggableType, useRecordTags } from "../lib/tags.queries";
import type { SectionFlow } from "./sectionedit";
import { TagsPanel } from "./tagspanel";

type TaggedRecord = WritableRecord & Readonly<{ id: string }>;

/**
 * A record's Tags row in its details, read only: its tags change through
 * `useTagsFlow`. Only an answer with no tags is empty. Loading and withheld
 * are the panel's to say, and a failed read draws no row.
 */
export function useTagsRow(
  entityType: TaggableType,
  record: TaggedRecord,
): DetailRow[] {
  const t = useT();
  const archived = Boolean(record.archived_at);
  const tags = useRecordTags(entityType, record.id, !archived);
  const answered = tags.isSuccess && !tags.data.withheld;
  const tagged = (tags.data?.data ?? []).length > 0;
  const term = t("tags.panelTitle");
  if (archived || (answered && !tagged)) {
    return [{ key: "tags", term, empty: true, value: t("field.unset") }];
  }
  if (tags.isError) {
    return [];
  }
  return [
    {
      key: "tags",
      term,
      block: tagged,
      value: (
        <TagsPanel
          entityType={entityType}
          entityID={record.id}
          bare
          canEdit={false}
        />
      ),
    },
  ];
}

/**
 * The Tags row of a section's dialog, which opens the tags' own editor. No
 * flow for a reader who may not change the record, or one held read-only.
 */
export function useTagsFlow(
  entityType: TaggableType,
  record: TaggedRecord,
  readOnly = false,
): Readonly<{ flow: SectionFlow | undefined; element: ReactNode }> {
  const t = useT();
  const [open, setOpen] = useState(false);
  const archived = Boolean(record.archived_at);
  const tags = useRecordTags(entityType, record.id, !archived);
  const mayTag = useCanWriteRecord(entityType, record) && !archived;
  if (!mayTag || readOnly) {
    return { flow: undefined, element: null };
  }
  const names = tags.data?.withheld
    ? undefined
    : (tags.data?.data ?? []).map((tag) => tag.name).join(", ");
  return {
    flow: {
      label: t("tags.panelTitle"),
      reading: names || undefined,
      open: () => setOpen(true),
    },
    element: (
      <TagsDrawer
        entityType={entityType}
        entityID={record.id}
        open={open}
        onClose={() => setOpen(false)}
      />
    ),
  };
}

// The tags' own editor, adding and removing, over the details that read them.
function TagsDrawer({
  entityType,
  entityID,
  open,
  onClose,
}: Readonly<{
  entityType: TaggableType;
  entityID: string;
  open: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const titleId = useId();
  return (
    <Modal open={open} onClose={onClose} labelledBy={titleId} intent="drawer">
      <DrawerHead>
        <Heading size="large" id={titleId} className="t-h2 modal-title">
          {t("tags.panelTitle")}
        </Heading>
      </DrawerHead>
      <DrawerBody>
        <TagsPanel entityType={entityType} entityID={entityID} bare canEdit />
      </DrawerBody>
    </Modal>
  );
}
