// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { type ReactNode, useState } from "react";
import { EntityRef } from "../../components/entityref";
import type { SectionFlow } from "../../components/sectionedit";
import { useTagsFlow } from "../../components/tagsflow";
import { useT } from "../../i18n";
import { LeadProductName, LeadProductPicker } from "./leadproduct";
import { LeadProjectPicker } from "./leadraillinks";
import type { Lead, LeadWriter } from "./leads";

type PickerFlow = "project" | "product";

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
  const tags = useTagsFlow("lead", lead, writer.readOnly);
  if (writer.readOnly) {
    return { flows: {}, element: null };
  }
  const close = () => setOpen(null);
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
    ...(tags.flow ? { tags: tags.flow } : {}),
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
        {tags.element}
      </>
    ),
  };
}
