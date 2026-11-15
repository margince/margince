import { useQuery } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { CreateAction, type CreateField } from "../../components/create";
import { currencyField } from "../../components/currencyfield";
import { toMinorUnits } from "../../format/minorunits";
import { useLocale, useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { currencyBeside } from "../../lib/dealcurrency";
import { translatedPipeline } from "../../lib/labeltranslation";
import { useLeadingCurrencies } from "../../lib/leadingcurrencies";
import {
  defaultPipelineOf,
  pipelinesOptions,
} from "../../lib/pipelines.queries";
import { unwrap } from "../../lib/problem";
import { useProjectCreateForm } from "../projects/projects.create";

/**
 * NewDealAction opens a deal on THIS company.
 *
 * The company is not a field: the rep is standing on the record, so
 * asking them to name it again is asking them to confirm where they already
 * are — and it is the one value they could get wrong.
 */
export function NewDealAction({
  companyId,
  companyName,
  projectId,
  onCreated,
  link,
}: Readonly<{
  companyId: string;
  companyName: string;
  // Drawn as a card's header link rather than a button.
  link?: boolean;
  // The project the new deal is born into, when the verb is offered from a
  // project page. The deal then names the project at birth, which is the only
  // moment the deal and the project are guaranteed to share a company.
  projectId?: string;
  onCreated?: (deal: components["schemas"]["Deal"]) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const target = useQuery({ ...pipelinesOptions, select: defaultPipelineOf });
  const leading = useLeadingCurrencies();
  const pipeline = target.data && translatedPipeline(target.data, locale);
  const open =
    pipeline?.stages?.filter((stage) => stage.semantic === "open") ?? [];
  // No pipeline, or none of its stages is open, means there is nowhere for a
  // deal to land. A button that can only fail is worse than no button.
  if (!pipeline || open.length === 0) {
    return null;
  }

  const fields: CreateField[] = [
    { key: "name", label: "create.dealName", required: true },
    {
      key: "amount",
      label: "create.amount",
      type: "number",
      step: "0.01",
      min: "0",
      wireField: "amount_minor",
    },
    currencyField(leading, locale),
    {
      key: "stage_id",
      label: "create.stage",
      type: "select",
      required: true,
      options: open.map((stage) => ({ value: stage.id, label: stage.name })),
    },
    { key: "expected_close_date", label: "create.expectedClose", type: "date" },
  ];

  const createDeal = async (
    values: Record<string, string>,
    _rows: unknown,
    idempotencyKey: string,
  ) => {
    const amount = values.amount?.trim();
    const currency = values.currency || leading[0];
    return unwrap(
      await api.POST("/deals", {
        params: { header: { "Idempotency-Key": idempotencyKey } },
        body: {
          name: (values.name ?? "").trim(),
          pipeline_id: pipeline.id,
          stage_id: values.stage_id ?? "",
          // The form takes major units; the wire is minor units.
          amount_minor: amount ? toMinorUnits(Number(amount), currency) : null,
          currency: currencyBeside(currency, amount),
          company_id: companyId,
          project_id: projectId ?? null,
          expected_close_date: values.expected_close_date || null,
          source: "manual",
        },
      }),
      t,
    );
  };

  return (
    <CreateAction
      label={t("co.deal.new", { name: companyName })}
      // The page that offered the verb is the page that has to show the deal.
      invalidate={projectId ? "project" : "company360"}
      screen="deals"
      create={createDeal}
      fields={fields}
      aboutId={companyId}
      stay={onCreated !== undefined}
      onCreated={onCreated}
      link={link}
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
  companyId,
  companyName,
}: Readonly<{ companyId: string; companyName: string }>) {
  // The project grant and the seat, not the company's own writability: the
  // write lands on a new project, and the company is only named on it.
  const canCreate = useCanWrite("project", "create");
  return canCreate ? (
    <NewProjectDialog companyId={companyId} companyName={companyName} />
  ) : null;
}

// Mounted only behind the grant, so a reader who may not create a project
// never reads the project custom-field catalog the form carries.
function NewProjectDialog({
  companyId,
  companyName,
}: Readonly<{ companyId: string; companyName: string }>) {
  const t = useT();
  const form = useProjectCreateForm({ pinned: companyId });
  return (
    <CreateAction
      label={t("co.project.new", { name: companyName })}
      // The record that offered the verb is the record that has to show the
      // project.
      invalidate="company360"
      screen="projects"
      create={form.create}
      onValuesChange={form.onValuesChange}
      fields={form.fields}
      aboutId={companyId}
    />
  );
}
