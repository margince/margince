// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useId, useState } from "react";
import { EntityRef } from "../../components/entityref";
import type { SectionFlow } from "../../components/sectionedit";
import { TagsPanel } from "../../components/tagspanel";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { Heading } from "../../design-system/heading";
import { Modal } from "../../design-system/modal";
import { useT } from "../../i18n";
import { useCanWriteRecord } from "../../lib/capability";
import { useRecordTags } from "../../lib/tags.queries";
import { LeadProductName, LeadProductPicker } from "./leadproduct";
import { LeadProjectPicker } from "./leadraillinks";
import type { Lead, LeadWriter } from "./leads";

type PickerFlow = "project" | "product" | "tags";

/**
 * The rows a lead changes through a picker of their own: its project, its
 * product and its tags. None for a lead nobody may change.
 */
export function useLeadPickerFlows(
  lead: Lead,
  writer: LeadWriter,
): Readonly<{ flows: Record<string, SectionFlow>; element: ReactNode }> {
  const t = useT();
  const [open, setOpen] = useState<PickerFlow | null>(null);
  const tags = useRecordTags("lead", lead.id, !lead.archived_at);
  const mayTag = useCanWriteRecord("lead", lead);
  if (writer.readOnly) {
    return { flows: {}, element: null };
  }
  const close = () => setOpen(null);
  const tagNames = tags.data?.withheld
    ? undefined
    : (tags.data?.data ?? []).map((tag) => tag.name).join(", ");
  const flows: Record<string, SectionFlow> = {
    project_id: {
      label: t("lead.project"),
      reading: lead.project_id ? (
        <EntityRef kind="project" id={lead.project_id} asText />
      ) : undefined,
      open: () => setOpen("project"),
    },
    product_id: {
      label: t("lead.product"),
      reading: <LeadProductName lead={lead} />,
      open: () => setOpen("product"),
    },
    ...(mayTag
      ? {
          tags: {
            label: t("tags.panelTitle"),
            reading: tagNames || undefined,
            open: () => setOpen("tags"),
          },
        }
      : {}),
  };
  return {
    flows,
    element: (
      <>
        <LeadProjectPicker
          lead={lead}
          writer={writer}
          open={open === "project"}
          onClose={close}
        />
        <LeadProductPicker
          lead={lead}
          writer={writer}
          open={open === "product"}
          onClose={close}
        />
        {mayTag && (
          <LeadTagsDrawer lead={lead} open={open === "tags"} onClose={close} />
        )}
      </>
    ),
  };
}

// The tags' own editor, adding and removing, over the details that read them.
function LeadTagsDrawer({
  lead,
  open,
  onClose,
}: Readonly<{ lead: Lead; open: boolean; onClose: () => void }>) {
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
        <TagsPanel entityType="lead" entityID={lead.id} bare canEdit />
      </DrawerBody>
    </Modal>
  );
}
