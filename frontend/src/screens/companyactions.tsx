import { useQuery } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { toMinorUnits } from "../format/minorunits";
import { useT } from "../i18n";
import { throwProblem } from "./common";
import { CreateAction, type CreateField } from "./create";
import { mapProjectCreate } from "./projects.form";

// What a rep can START from the company page.
//
// The page could already say everything about an account and do nothing with
// it: opening a deal meant leaving for the pipeline board and re-finding the
// company there, and applying a tag or adding the account to a list had no
// control at all. Each action below is the record's OWN verb — the company is
// the subject, so it is never something the rep has to pick.

type Pipeline = components["schemas"]["Pipeline"];
type Stage = components["schemas"]["Stage"];

/**
 * useDealTarget resolves where a deal created from this page should land: the
 * default pipeline and its first OPEN stage.
 *
 * Open only. A deal is born open (INV-CLOSE-PAST), so won and lost are reached
 * through the confirmed advance and must never be a create-time choice — the
 * deal board applies the same rule to its own stage picker.
 */
function useDealTarget() {
  return useQuery({
    queryKey: ["pipelines", "dealTarget"],
    queryFn: async () => {
      const { data, error } = await api.GET("/pipelines", {
        params: { query: {} },
      });
      if (error) {
        throwProblem(error);
      }
      const pipeline: Pipeline | undefined =
        data.data.find((candidate) => candidate.is_default) ?? data.data[0];
      const stages: Stage[] = pipeline?.stages ?? [];
      const open = stages.filter((stage) => stage.semantic === "open");
      return { pipeline, open };
    },
  });
}

/**
 * NewDealAction opens a deal on THIS company.
 *
 * The organization is not a field: the rep is standing on the record, so
 * asking them to name it again is asking them to confirm where they already
 * are — and it is the one value they could get wrong.
 */
export function NewDealAction({
  orgId,
  orgName,
  projectId,
}: Readonly<{
  orgId: string;
  orgName: string;
  // The project the new deal is born into, when the verb is offered from a
  // project page. The deal then names the project at birth, which is the only
  // moment the deal and the project are guaranteed to share a company.
  projectId?: string;
}>) {
  const t = useT();
  const target = useDealTarget();
  const pipeline = target.data?.pipeline;
  const open = target.data?.open ?? [];
  // No pipeline, or none of its stages is open, means there is nowhere for a
  // deal to land. A button that can only fail is worse than no button.
  if (!pipeline || open.length === 0) {
    return null;
  }

  const fields: CreateField[] = [
    { key: "name", label: "create.dealName", required: true },
    { key: "amount", label: "create.amount", type: "number", step: "0.01" },
    {
      key: "currency",
      label: "create.currency",
      type: "select",
      required: true,
      options: ["EUR", "USD", "GBP", "CHF"].map((code) => ({
        value: code,
        label: code,
      })),
    },
    {
      key: "stage_id",
      label: "create.stage",
      type: "select",
      required: true,
      options: open.map((stage) => ({ value: stage.id, label: stage.name })),
    },
    { key: "expected_close_date", label: "create.expectedClose", type: "date" },
  ];

  const createDeal = async (values: Record<string, string>) => {
    const amount = values.amount?.trim();
    const { data, error } = await api.POST("/deals", {
      body: {
        name: values.name.trim(),
        pipeline_id: pipeline.id,
        stage_id: values.stage_id,
        // The form takes major units; the wire is minor units.
        //
        // Amount and currency travel together or not at all — the server
        // refuses a half-populated pair (amount_currency_pair), so sending a
        // currency beside an empty amount 422s the field the form presents as
        // optional.
        amount_minor: amount
          ? toMinorUnits(Number(amount), values.currency || "EUR")
          : null,
        currency: amount ? values.currency || "EUR" : null,
        organization_id: orgId,
        project_id: projectId ?? null,
        expected_close_date: values.expected_close_date || null,
        source: "manual",
      },
    });
    if (error) {
      throwProblem(error, t);
    }
    return data;
  };

  return (
    <CreateAction
      label={t("co.deal.new", { name: orgName })}
      // The page that offered the verb is the page that has to show the deal.
      invalidate={projectId ? "project" : "organization360"}
      screen="deals"
      create={createDeal}
      fields={fields}
      aboutId={orgId}
    />
  );
}

/**
 * NewProjectAction opens a project on THIS company.
 *
 * The company is not a field, for the same reason the deal's is not: the rep is
 * standing on the record, and the one value they could get wrong is the one
 * they do not have to give.
 *
 * The owner is not a field either. The server stamps the creator, and the only
 * choices a create form could offer resolve to the creator anyway; reassigning
 * is an edit, on the project's own page.
 */
export function NewProjectAction({
  orgId,
  orgName,
}: Readonly<{ orgId: string; orgName: string }>) {
  const t = useT();
  const fields: CreateField[] = [
    { key: "name", label: "project.name", required: true },
    { key: "description", label: "project.description", type: "textarea" },
    { key: "target_end_date", label: "project.targetEnd", type: "date" },
  ];
  const createProject = async (values: Record<string, string>) => {
    const { data, error } = await api.POST("/projects", {
      // Through the shared mapper, never a body written here: the projects
      // screen creates the same record, and two spellings of one request are
      // how the two come to disagree about an empty date.
      body: mapProjectCreate({ ...values, organization_id: orgId }),
    });
    if (error) {
      throwProblem(error, t);
    }
    return data;
  };
  return (
    <CreateAction
      label={t("co.project.new", { name: orgName })}
      // The record that offered the verb is the record that has to show the
      // project.
      invalidate="organization360"
      screen="projects"
      create={createProject}
      fields={fields}
      aboutId={orgId}
    />
  );
}
