import { useEntityName } from "../../components/entityref";
import { RecordFields } from "../../components/recordfields";
import { useT } from "../../i18n";
import { RecordCustomFields } from "../records/recordcustomfields";
import { saveRecordEdit } from "../records/recordedit";
import {
  searchProjectReferences,
  useRecordOwners,
} from "../records/recordreferences";
import { leadStatusLabel } from "./leadpresentation";
import type { Lead, LeadWriter } from "./leads";
import { leadEditFields, mapLeadUpdate } from "./leads.form";
import { sourcePickOptions, useLeadSources } from "./leadsources";

/**
 * The lead's own words, editable where they stand.
 *
 * The Edit modal is four clicks and a context switch to fix a misspelled
 * company name. It edits a lead wholesale and reaches the fields this grid does
 * not carry. The four a rep corrects while reading are corrected here.
 *
 * Every row saves through the same patch the lifecycle card uses. So two inline
 * edits cannot invalidate different caches or send a different If-Match.
 */
export function LeadIdentityFields({
  lead,
  writer,
}: Readonly<{ lead: Lead; writer: LeadWriter }>) {
  const t = useT();
  const project = useEntityName("project", lead.project_id);
  const sources = useLeadSources();
  const owners = useRecordOwners(lead.owner_id);
  const connector = (lead.source ?? "").startsWith("connector:");
  return (
    <>
      <RecordFields
        title={t("lead.details")}
        kind="lead"
        links={
          lead.linkedin_url
            ? {
                linkedin_url: {
                  href: lead.linkedin_url,
                  label: t("record.openProfile"),
                },
              }
            : undefined
        }
        record={lead}
        fields={[
          ...leadEditFields,
          {
            key: "owner_id",
            label: "list.owner",
            type: "select",
            options: owners,
            required: true,
          },
          {
            key: "status",
            label: "history.field.status",
            toInput: () => {
              const label = leadStatusLabel(lead.status);
              return label ? t(label) : lead.status;
            },
          },
          { key: "score", label: "lead.score", type: "number" },
          { key: "linkedin_url", label: "create.linkedinUrl" },
          { key: "candidate_company_key", label: "record.companyRoutingKey" },
          {
            key: "project_id",
            label: "lead.project",
            searchTargets: searchProjectReferences,
            options: lead.project_id
              ? [
                  {
                    value: lead.project_id,
                    label: project.name ?? lead.project_id,
                  },
                ]
              : [],
          },
          {
            key: "source",
            label: "lead.source",
            type: "select",
            required: true,
            options: sourcePickOptions(sources.data?.data, lead.source, t),
          },
        ]}
        groups={[{ label: t("create.email"), keys: ["email"] }]}
        canEdit={!writer.readOnly}
        readOnlyFields={{
          linkedin_url: t("record.leadProfileReadOnly"),
          status: t("record.leadStatusAction"),
          score: t("record.leadScoreAction"),
          ...(connector ? { source: t("lead.sourceFromConnector") } : {}),
        }}
        resolveExisting={(_code, id) => ({ screen: "leads", id })}
        save={async (values, _rows, opened) =>
          saveRecordEdit("lead", opened, mapLeadUpdate(values))
        }
      />
      <RecordCustomFields kind="lead" record={lead} />
    </>
  );
}
