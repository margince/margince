import { useState } from "react";
import type { CreateField } from "../../components/create";
import { useEntityName } from "../../components/entityref";
import { RecordFields, rawRecord } from "../../components/recordfields";
import { useT } from "../../i18n";
import { useCanWriteRecord } from "../../lib/capability";
import { useAcquisitionSources } from "../../screens/acquisitionsources.queries";
import { RecordCustomFields } from "../../screens/recordcustomfields";
import { saveRecordEdit } from "../../screens/recordedit";
import { RecordListsPanel } from "../../screens/recordlists";
import {
  useOwnerPicker,
  useRecordOwners,
} from "../../screens/recordreferences";
import { RecordTeam } from "../../screens/recordteam";
import { TagsPanel } from "../lists/tagspanel";
import { DealEmailAside } from "./dealemail";
import {
  NEW_PROJECT_OWNER,
  resolveDealProject,
  useProjectsOfCompany,
} from "./dealproject";
import type { Deal } from "./deals";
import {
  attributedPartner,
  dealEditFields,
  editProjectFields,
  usePartnerOptions,
} from "./deals.fields";
import { dealEditRecord, mapDealUpdate, stringValues } from "./deals.write";

export function DealDetails({
  deal,
  companies,
  meId,
}: Readonly<{
  deal: Deal;
  companies: { id: string; display_name: string }[];
  meId: string;
}>) {
  const t = useT();
  const partnerOptions = usePartnerOptions(companies);
  const acquisitionSources = useAcquisitionSources().data;
  const masked = deal.masked_fields ?? [];
  const companyOnPage = companies.find(
    (company) => company.id === deal.company_id,
  );
  const companyById = useEntityName(
    "company",
    companyOnPage ? null : deal.company_id,
  );
  const currentCompany = deal.company_id
    ? {
        id: deal.company_id,
        label:
          companyOnPage?.display_name ?? companyById.name ?? deal.company_id,
      }
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
        fields={[
          ...dealEditFields(t, {
            companies,
            partnerOptions,
            attributedPartner: attributedPartner(deal, companies),
            currentCompany,
            masked,
            me: meId,
            currentOwner: deal.owner_id ?? null,
            currency: deal.currency ?? "",
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
      canEdit={canUpdate && !deal.archived_at}
    />
  );
}

// The deal's context, for the details pane. First what the deal is and who of
// ours owns it, then how the deal is filed and the mail card.
export function DealContext({
  deal,
  companies,
  meId,
}: Readonly<{
  deal: Deal;
  companies: { id: string; display_name: string }[];
  meId: string;
}>) {
  // The same per-row answer the deal's other verbs read. An archived deal
  // takes no new responsibilities, and neither does one this seat may read
  // but not write.
  const canWrite = useCanWriteRecord("deal", deal) && !deal.archived_at;
  return (
    <>
      {/* What the deal is commercially, then who of ours is on it. The buyer's
          seats are not here: they live in the committee card in the column, with
          the verbs that change them. The rail holds no second copy of that list. */}
      <DealDetails deal={deal} companies={companies} meId={meId} />
      <RecordTeam recordType="deal" recordId={deal.id} readOnly={!canWrite} />
      <DealTagsSection deal={deal} />
      <RecordListsPanel entityType="deal" entityId={deal.id} />
      <DealEmailAside dealId={deal.id} />
    </>
  );
}
