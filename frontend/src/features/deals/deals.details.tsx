import { useId, useState } from "react";
import type { CreateField } from "../../components/create";
import { useEntityName } from "../../components/entityref";
import { RecordCustomFields } from "../../components/recordcustomfields";
import { saveRecordEdit } from "../../components/recordedit";
import { RecordFields } from "../../components/recordfields";
import { rawRecord } from "../../components/recordfieldtypes";
import { RecordTeam } from "../../components/recordteam";
import { TagsPanel } from "../../components/tagspanel";
import { DrawerBody, DrawerHead } from "../../design-system/drawerbands";
import { Heading } from "../../design-system/heading";
import { Modal } from "../../design-system/modal";
import { useLocale, useT } from "../../i18n";
import { useAcquisitionSources } from "../../lib/acquisitionsources.queries";
import { useCanWriteRecord } from "../../lib/capability";
import { useLeadingCurrencies } from "../../lib/leadingcurrencies";
import { RecordListsPanel } from "../records/recordlists";
import { useOwnerPicker, useRecordOwners } from "../records/recordreferences";
import { DealRecordRows } from "./deal360/dealheaderfacts";
import {
  NEW_PROJECT_OWNER,
  resolveDealProject,
  useProjectsOfCompany,
} from "./dealproject";
import type { Deal, Stage } from "./deals";
import {
  attributedPartner,
  dealEditFields,
  editProjectFields,
  usePartnerOptions,
} from "./deals.fields";
import { dealEditRecord, mapDealUpdate, stringValues } from "./deals.write";
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
  const partnerOptions = usePartnerOptions([]);
  const acquisitionSources = useAcquisitionSources().data;
  const leadingCurrencies = useLeadingCurrencies();
  const masked = deal.masked_fields ?? [];
  const companyName = useEntityName("company", deal.company_id).name;
  const partnerName = useEntityName("company", deal.partner_company_id).name;
  const currentCompany = deal.company_id
    ? { id: deal.company_id, label: companyName ?? deal.company_id }
    : undefined;
  const [formValues, setFormValues] = useState<Record<string, string>>({});
  const formCompany = formValues.company_id ?? deal.company_id ?? undefined;
  const openProjects = useProjectsOfCompany(formCompany);
  const projectById = useEntityName("project", deal.project_id);
  const currentProject = deal.project_id
    ? { id: deal.project_id, label: projectById.name ?? deal.project_id }
    : undefined;
  const projectOwner = useOwnerPicker(null);
  const seeded = { ...dealEditRecord(deal), original: deal };
  const canEdit = useCanWriteRecord("deal", deal) && !deal.archived_at;
  const owners = useRecordOwners(deal.owner_id);
  // Company/project and money/currency are coupled writes: changing a visible
  // member must not silently rewrite a reference or unit the reader cannot see.
  const readOnlyFields: Record<string, string> = Object.fromEntries(
    masked.map((key) => [
      key === "amount_minor"
        ? "amount"
        : key === "expected_arr_minor"
          ? "expected_arr"
          : key,
      t("record.notShown"),
    ]),
  );
  if (deal.arr_source_offer_id)
    Object.assign(readOnlyFields, {
      expected_arr: t("deal.arrFromOffer"),
      currency: t("deal.arrFromOffer"),
    });
  return (
    <>
      <RecordFields
        kind="deal"
        onValuesChange={setFormValues}
        canEdit={canEdit}
        groups={[
          {
            label: t(
              deal.arr_source_offer_id ? "deal.expectedArr" : "create.amount",
            ),
            keys: deal.arr_source_offer_id
              ? ["expected_arr", "currency"]
              : ["amount", "expected_arr", "currency"],
          },
          {
            label: t("create.relatedCompany"),
            keys: [
              "company_id",
              "project_id",
              "new_project_name",
              NEW_PROJECT_OWNER,
            ],
          },
          {
            label: t("deal.partnerCompany"),
            keys: ["partner_company_id", "partner_attribution"],
          },
        ]}
        readOnlyFields={readOnlyFields}
        maskedFields={masked
          .filter((key) =>
            ["amount_minor", "expected_arr_minor", "currency"].includes(key),
          )
          .map((key) =>
            key === "amount_minor"
              ? "amount"
              : key === "expected_arr_minor"
                ? "expected_arr"
                : key,
          )}
        title={t("co.details.title")}
        extraRows={
          <DealRecordRows deal={deal} stages={stages} locale={locale} />
        }
        fields={[
          ...dealEditFields(t, {
            partnerOptions,
            attributedPartner: attributedPartner(deal, partnerName),
            currentCompany,
            masked,
            me: meId,
            currentOwner: deal.owner_id ?? null,
            currency: deal.currency ?? "",
            leadingCurrencies,
            acquisitionSources,
            currentAcquisitionSource: deal.acquisition_source,
          }).map(
            (field): CreateField =>
              field.key === "owner_id"
                ? { ...field, label: "list.owner", options: owners }
                : field,
          ),
          ...editProjectFields(t, {
            masked,
            openProjects,
            currentProject:
              formCompany === deal.company_id ? currentProject : undefined,
            company: formCompany,
            owner: projectOwner,
          }),
        ]}
        record={seeded}
        save={async (values, _rows, opened) => {
          const submitted = stringValues({ ...opened, ...values });
          const projectId = Object.hasOwn(values, "project_id")
            ? await resolveDealProject(
                submitted,
                submitted.company_id?.trim() || null,
                t,
              )
            : submitted.project_id;
          return saveRecordEdit(
            "deal",
            rawRecord(opened),
            mapDealUpdate(
              { ...opened, ...values, project_id: projectId ?? "" },
              opened,
              masked,
            ),
          );
        }}
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
