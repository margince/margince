import { useId } from "react";
import { RecordCustomFields } from "../../components/recordcustomfields";
import { RecordFields } from "../../components/recordfields";
import { RecordTeam } from "../../components/recordteam";
import { TagsPanel } from "../../components/tagspanel";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { Heading } from "../../design-system/heading";
import { Modal } from "../../design-system/modal";
import { useLocale, useT } from "../../i18n";
import { useCanWriteRecord } from "../../lib/capability";
import { RecordListsPanel } from "../records/recordlists";
import { DealRecordRows } from "./deal360/dealheaderfacts";
import { useDealEdit } from "./dealedit";
import type { Deal, Stage } from "./deals";
import { ExtensionPanels } from "./extensionpanels";

export function DealDetails({
  deal,
  meId,
  stages,
}: Readonly<{
  deal: Deal;
  meId: string;
  stages: readonly Stage[];
}>) {
  const t = useT();
  const { locale } = useLocale();
  const edit = useDealEdit(deal, meId);
  return (
    <>
      <RecordFields
        kind="deal"
        onValuesChange={edit.onValuesChange}
        canEdit={edit.canEdit}
        groups={edit.groups}
        readOnlyFields={edit.readOnlyFields}
        maskedFields={edit.maskedFields}
        title={t("co.details.title")}
        extraRows={
          <DealRecordRows deal={deal} stages={stages} locale={locale} />
        }
        fields={[...edit.fields]}
        record={edit.record}
        save={edit.save}
      />
      <RecordCustomFields kind="deal" record={deal} />
    </>
  );
}

/**
 * The deal's tags, drawn by the shared panel.
 *
 * It asks the three questions the server asks before it writes. They are the
 * object grant, this record's own editability, and (inside the panel) whether
 * the caller may see the vocabulary at all.
 */
function DealTagsSection({ deal }: Readonly<{ deal: Deal }>) {
  // useCanWriteRecord, not useCan: applying a tag writes to the deal. So the
  // verb owes the seat ceiling and this row's own `writable` beside the object
  // grant. A grant alone would offer the picker on a colleague's deal, and to a
  // read seat the server refuses before RBAC is consulted.
  const canUpdate = useCanWriteRecord("deal", deal);
  return (
    <TagsPanel
      entityType="deal"
      entityID={deal.id}
      archived={Boolean(deal.archived_at)}
      canEdit={canUpdate && !deal.archived_at}
    />
  );
}

/**
 * Every field of the deal in one drawer, with the editors that change them.
 * They cover the fields, custom fields, who of ours is on it, its tags and its lists.
 * The page's left column reads these; this is where they are written.
 */
export function DealDetailsDrawer({
  deal,
  meId,
  stages,
  open,
  onClose,
}: Readonly<{
  deal: Deal;
  meId: string;
  stages: readonly Stage[];
  open: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const headingId = useId();
  // An archived deal takes no new responsibilities, and neither does one this
  // seat may read but not write.
  const canWrite = useCanWriteRecord("deal", deal) && !deal.archived_at;
  return (
    <Modal open={open} onClose={onClose} labelledBy={headingId} intent="drawer">
      <DrawerHead>
        <Heading size="large" id={headingId} className="t-h2 modal-title">
          {t("deal.editDetails")}
        </Heading>
      </DrawerHead>
      <DrawerBody className="form-stack">
        <DealDetails deal={deal} meId={meId} stages={stages} />
        <RecordTeam recordType="deal" recordId={deal.id} readOnly={!canWrite} />
        <DealTagsSection deal={deal} />
        <RecordListsPanel
          entityType="deal"
          entityId={deal.id}
          archived={Boolean(deal.archived_at)}
        />
        {/* Units' panels close the column, grouped under their own label: each
            is an add-on's card about this deal, fenced off so its failure stays
            there. */}
        <ExtensionPanels
          slot="record_panel.deal"
          record={{ type: "deal", id: deal.id }}
        />
      </DrawerBody>
    </Modal>
  );
}
