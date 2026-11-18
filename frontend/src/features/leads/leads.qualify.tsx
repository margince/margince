import {
  keepPreviousData,
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import {
  Checkbox,
  Field,
  Textarea,
  TextInput,
} from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { Heading } from "../../design-system/heading";
import { MoneyInput } from "../../design-system/moneyinput";
import { Select } from "../../design-system/select";
import { formatMoney } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import {
  invalidateClosedLead,
  leadPromotePreviewKey,
} from "../../lib/leadkeys";
import { defaultPipelineOf, usePipelines } from "../../lib/pipelines.queries";
import { unwrap } from "../../lib/problem";
import { useInstallationSettings } from "../../lib/uploadlimit";
import { useViewerZone } from "../../lib/viewerzone";
import { useLeadCompanyNameReading } from "./leadcompany";
import { leadIdentityName } from "./leadname";
import {
  amountState,
  dealBlock,
  ExistingQualificationDeal,
  openStagesOf,
  type QualificationDealChoice,
  qualificationConfirmState,
  qualificationDealState,
  qualifyRefusals,
} from "./leads.qualifydeal";
import { reasonSentence, triggerFor } from "./leads.qualifyreason";
import "./leads.css";

type Lead = components["schemas"]["Lead"];
type PromoteLeadRequest = components["schemas"]["PromoteLeadRequest"];
type PromoteLeadResponse = components["schemas"]["PromoteLeadResponse"];

// The qualify dialog: "this lead is now a contact, and maybe an opportunity".
// Three blocks — what happens to the contact (read from the server's own
// preview), an optional deal opened in the same transaction, and why: the
// reason is DERIVED from what was captured (the strongest engagement signal)
// rather than asked for as a technical trigger; a rep who qualifies a lead
// nobody has heard from is recorded as the human who decided it.

// The server's preview of a promotion, read without writing. An absent contact
// on a `merge` means the match is outside the reader's row scope, not "no match".
export function usePromotePreview(
  id: string,
  open: boolean,
  createCompany = false,
) {
  const t = useT();
  return useInfiniteQuery({
    queryKey: [...leadPromotePreviewKey(id), { createCompany }],
    initialPageParam: "",
    getNextPageParam: (page: components["schemas"]["PromoteLeadPreview"]) =>
      page.existing_deals_cursor,
    select: (data) => {
      const first = data.pages[0];
      if (!first) return undefined;
      return {
        ...first,
        existing_deals: data.pages.flatMap((page) => page.existing_deals ?? []),
        existing_deals_cursor: data.pages.at(-1)?.existing_deals_cursor,
      };
    },
    enabled: open,
    placeholderData: keepPreviousData,
    // Always fresh: the answer is about the workspace as it stands the moment
    // the dialog opens, and a 30s-old "create" can be a merge by now.
    staleTime: 0,
    queryFn: async ({
      pageParam,
      signal,
    }): Promise<components["schemas"]["PromoteLeadPreview"]> => {
      return unwrap(
        await api.GET("/leads/{id}/promote-preview", {
          params: {
            path: { id },
            query: {
              create_company: createCompany,
              deals_cursor: pageParam || undefined,
            },
          },
          signal,
        }),
        t,
      );
    },
  });
}

// triggerFor reads the wire trigger off the lead's captured evidence; a lead
// with none is qualified by the human's own judgement.
export { triggerFor } from "./leads.qualifyreason";

type QualifyDialogProps = Readonly<{
  lead: Lead;
  open: boolean;
  onClose: () => void;
  onQualified: (result: PromoteLeadResponse) => void;
}>;

export function QualifyDialog(props: QualifyDialogProps) {
  return props.open ? <OpenQualificationDialog {...props} /> : null;
}

function OpenQualificationDialog(props: QualifyDialogProps) {
  const t = useT();
  const [createCompany, setCreateCompany] = useState(false);
  const preview = usePromotePreview(props.lead.id, props.open, createCompany);
  if (!props.open) return null;
  if (!preview.data)
    return (
      <ConfirmModal
        open
        onClose={props.onClose}
        intent="form"
        title={t("lead.qualify.title", { name: leadIdentityName(props.lead) })}
        confirmLabel={t("lead.qualify.confirm")}
        confirmDisabled
        onConfirm={() => undefined}
      >
        <PreviewSentence preview={preview} t={t} />
      </ConfirmModal>
    );
  return (
    <QualificationForm
      key={props.lead.id}
      {...props}
      preview={preview}
      createCompany={createCompany}
      setCreateCompany={setCreateCompany}
    />
  );
}

function QualificationForm({
  lead,
  open,
  onClose,
  onQualified,
  preview,
  createCompany,
  setCreateCompany,
}: QualifyDialogProps & {
  preview: ReturnType<typeof usePromotePreview>;
  createCompany: boolean;
  setCreateCompany: (value: boolean) => void;
}) {
  const t = useT();
  const { locale } = useLocale();
  // The signal is an activity, so its day is the record's.
  const zone = useViewerZone();
  const queryClient = useQueryClient();
  const pipelines = usePipelines(open);
  // The money pair holds from birth: an amount travels with the
  // installation's base currency, the only one a qualify dialog can name.
  const installation = useInstallationSettings();
  const currency = installation.data?.base_currency;
  // An engaged lead is one a rep has something to sell to, so the deal
  // block starts ticked for it and unticked otherwise.
  const [dealChoice, setDealChoice] = useState<QualificationDealChoice>(() => {
    const initial = qualificationDealState(
      lead.status,
      null,
      preview.data,
      preview.hasNextPage,
    );
    return initial.selectedDeal
      ? { id: initial.selectedDeal.id, company: preview.data?.company_id }
      : initial.withDeal
        ? "new"
        : "none";
  });
  const { selectedDeal, withDeal, invalidSelection } = qualificationDealState(
    lead.status,
    dealChoice,
    preview.data,
    preview.hasNextPage,
  );

  const [pipelineId, setPipelineId] = useState("");
  const [stageId, setStageId] = useState("");
  const [dealName, setDealName] = useState("");
  const [amountMinor, setAmountMinor] = useState<number | null>(null);
  const [note, setNote] = useState("");
  const typedCompany = preview.data?.typed_company;

  const defaultPipeline = defaultPipelineOf(pipelines.data ?? []);
  const chosenPipeline =
    pipelines.data?.find((p) => p.id === pipelineId) ?? defaultPipeline;
  const stages = openStagesOf(chosenPipeline);
  const chosenStage = stages.find((s) => s.id === stageId) ?? stages[0];
  // The linked company's own name, which a rename there may have moved away
  // from what the lead typed. The deal this dialog offers to open carries the
  // name into a record of its own, so the stale one would outlive the lead.
  const company = useLeadCompanyNameReading(lead);
  const companyName = company.name ?? leadIdentityName(lead);
  const suggestedName = chosenStage
    ? `${companyName} — ${chosenStage.name}`
    : companyName;

  const qualify = useMutation({
    mutationFn: async (body: PromoteLeadRequest) => {
      return unwrap(
        await api.POST("/leads/{id}/promote", {
          params: { path: { id: lead.id } },
          body,
        }),
        t,
      );
    },
    onSuccess: (result) => {
      // The history key is among them: the promotion WROTE the audit row the
      // page reads its outcome from.
      invalidateClosedLead(queryClient, lead.id);
      if (result.deal_id) {
        queryClient.invalidateQueries({ queryKey: ["deals"] });
      }
      onQualified(result);
    },
  });

  const close = () => {
    qualify.reset();
    setCreateCompany(false);
    setDealChoice(null);
    onClose();
  };

  const submit = () => {
    const trimmedNote = note.trim();
    qualify.mutate({
      trigger: triggerFor(lead),
      evidence: {
        activity_id: lead.qualification_evidence?.activity_id ?? null,
        note: trimmedNote || null,
      },
      // Only a box the reader saw ticked in this opening may mint a company.
      create_company: Boolean(typedCompany) && createCompany,
      existing_deal_id: selectedDeal?.id,
      deal: withDeal
        ? dealBlock({
            pipelineId: chosenPipeline?.id,
            stageId: chosenStage?.id,
            name: dealName.trim() || suggestedName,
            amountMinor,
            currency,
          })
        : undefined,
    });
  };

  const { amountInvalid, amountWaitsForCurrency } = amountState(
    amountMinor,
    currency,
  );
  const refused = qualifyRefusals(qualify.error, t);
  const confirmation = qualificationConfirmState(
    { selectedDeal, withDeal, invalidSelection },
    preview,
    { amountInvalid, amountWaitsForCurrency },
    { nameSettling: company.settling && !dealName.trim() },
  );
  const name = leadIdentityName(lead);

  return (
    <ConfirmModal
      open={open}
      onClose={close}
      intent="form"
      title={t("lead.qualify.title", { name })}
      confirmLabel={t(confirmation.label)}
      confirmDisabled={confirmation.disabled}
      onConfirm={submit}
      pending={qualify.isPending}
      error={qualify.isError ? refused.foot : undefined}
    >
      <section className="lead-qualify-block">
        <Heading size="xsmall" as="h3">
          {t("lead.qualify.contact")}
        </Heading>
        <PreviewSentence preview={preview} t={t} />
      </section>

      {typedCompany && (
        <TypedCompanyChoice
          company={typedCompany}
          checked={createCompany}
          onChange={setCreateCompany}
        />
      )}

      <ExistingQualificationDeal
        preview={preview.data}
        selected={selectedDeal}
        newDeal={withDeal}
        invalid={invalidSelection}
        more={preview.hasNextPage}
        fetching={preview.isFetchingNextPage}
        loadMore={() => preview.fetchNextPage()}
        choose={setDealChoice}
      />
      {withDeal && (
        <div className="lead-qualify-deal">
          <Field label={t("lead.qualify.pipeline")}>
            {(control) => (
              <Select
                {...control}
                value={chosenPipeline?.id ?? ""}
                onChange={(value) => {
                  setPipelineId(value);
                  setStageId("");
                }}
                options={(pipelines.data ?? []).map((p) => ({
                  value: p.id,
                  label: p.name,
                }))}
              />
            )}
          </Field>
          <Field label={t("lead.qualify.stage")}>
            {(control) => (
              <Select
                {...control}
                value={chosenStage?.id ?? ""}
                onChange={setStageId}
                options={stages.map((s) => ({
                  value: s.id,
                  label: s.name,
                }))}
              />
            )}
          </Field>
          <Field label={t("lead.qualify.dealName")}>
            {(control) => (
              <TextInput
                {...control}
                data-testid="lead-qualify-deal-name"
                value={dealName}
                placeholder={suggestedName}
                onChange={(event) => setDealName(event.target.value)}
              />
            )}
          </Field>
          <Field
            label={t("lead.qualify.amount", { currency: currency ?? "" })}
            hint={t("lead.qualify.amountHint")}
            error={
              amountInvalid
                ? t("create.amountNegative")
                : amountWaitsForCurrency
                  ? t("lead.qualify.amountNoCurrency")
                  : refused.amount
            }
          >
            {(control) => (
              <MoneyInput
                {...control}
                valueMinor={amountMinor ?? 0}
                blankWhenZero={amountMinor === null}
                currency={currency ?? ""}
                min={0}
                inputMode="decimal"
                display={
                  currency
                    ? (minor) => formatMoney(minor, currency, locale)
                    : undefined
                }
                onChangeMinor={(minor) =>
                  setAmountMinor(Number.isNaN(minor) ? null : minor)
                }
                onClear={() => setAmountMinor(null)}
              />
            )}
          </Field>
        </div>
      )}

      <section className="lead-qualify-block">
        <Heading size="xsmall" as="h3">
          {t("lead.qualify.why")}
        </Heading>
        <p>{reasonSentence(lead, t, locale, zone)}</p>
      </section>
      <Field label={t("lead.evidenceNote")}>
        {(control) => (
          <Textarea
            {...control}
            value={note}
            onChange={(event) => setNote(event.target.value)}
          />
        )}
      </Field>
    </ConfirmModal>
  );
}

// The rep adds a typed company the lead's email cannot place. Unticked, it
// lands on the contact as a company to link or create.
function TypedCompanyChoice({
  company,
  checked,
  onChange,
}: Readonly<{
  company: string;
  checked: boolean;
  onChange: (checked: boolean) => void;
}>) {
  const t = useT();
  return (
    <section className="lead-qualify-block">
      <Heading size="xsmall" as="h3">
        {t("lead.qualify.company")}
      </Heading>
      <p>{t("lead.qualify.companyUnresolved", { company })}</p>
      <Checkbox
        label={t("lead.qualify.createCompany", { company })}
        checked={checked}
        data-testid="lead-qualify-create-company"
        onChange={(event) => onChange(event.target.checked)}
      />
      <p className="t-caption">
        {t("lead.qualify.createCompanyHint", { company })}
      </p>
    </section>
  );
}

function PreviewSentence({
  preview,
  t,
}: Readonly<{
  preview: ReturnType<typeof usePromotePreview>;
  t: ReturnType<typeof useT>;
}>) {
  if (preview.isPending) {
    return <p>{t("lead.previewPending")}</p>;
  }
  // A failed preview does not block the qualification; the confirm still
  // runs the same ladder. It just cannot be described in advance.
  if (preview.isError || !preview.data) {
    return null;
  }
  if (preview.data.outcome === "create") {
    return <p>{t("lead.previewCreate")}</p>;
  }
  if (!preview.data.contact) {
    return <p>{t("lead.previewMergeWithheld")}</p>;
  }
  return (
    <p>
      {t("lead.previewMerge")}{" "}
      <EntityRef kind="contact" id={preview.data.contact.id} />
    </p>
  );
}
