import type { components } from "../../api/schema";
import { fieldRefusalSentence } from "../../components/create.refusals";
import { Field } from "../../design-system/atoms";
import { Button } from "../../design-system/button";
import { type Choice, ChoiceList } from "../../design-system/choicelist";
import { Select } from "../../design-system/select";
import { formatMoney } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";
import { problemFieldErrorsOf, problemMessageOf } from "../../lib/problem";

type Deal = components["schemas"]["Deal"];
type Preview = components["schemas"]["PromoteLeadPreview"];
type PromoteLeadRequest = components["schemas"]["PromoteLeadRequest"];
type Pipeline = components["schemas"]["Pipeline"];
export type QualificationDealChoice =
  | "new"
  | "none"
  | { id: string; company?: string }
  | null;

export function qualificationDealState(
  status: string,
  choice: QualificationDealChoice,
  preview: Preview | undefined,
  more: boolean,
) {
  const candidates = preview?.existing_deals ?? [];
  const unique = candidates.length === 1 && !more ? candidates[0] : undefined;
  const explicit = typeof choice === "object" && choice !== null;
  const selectedDeal = explicit
    ? candidates.find(
        (deal) =>
          deal.id === choice.id && choice.company === preview?.company_id,
      )
    : choice === null
      ? unique
      : undefined;
  return {
    selectedDeal,
    withDeal:
      choice === "new" || (choice === null && !unique && status === "engaged"),
    invalidSelection: explicit && !selectedDeal,
  };
}

export function ExistingQualificationDeal({
  preview,
  selected,
  newDeal,
  invalid,
  more,
  fetching,
  loadMore,
  choose,
}: Readonly<{
  preview: Preview | undefined;
  selected: Deal | undefined;
  newDeal: boolean;
  invalid: boolean;
  more: boolean;
  fetching: boolean;
  loadMore: () => void;
  choose: (choice: QualificationDealChoice) => void;
}>) {
  const t = useT();
  const candidates = preview?.existing_deals ?? [];
  const first = candidates[0];
  const { locale } = useLocale();
  const choices: Choice<"existing" | "new" | "none">[] = [
    ...(first
      ? [
          {
            value: "existing",
            label: t("lead.qualify.existingDeal"),
          } satisfies Choice<"existing">,
        ]
      : []),
    { value: "new", label: t("lead.qualify.newDeal") },
    { value: "none", label: t("lead.qualify.withoutDeal") },
  ];
  return (
    <>
      <section className="lead-qualify-block">
        <ChoiceList
          legend={t("lead.qualify.dealChoice")}
          choices={choices}
          value={
            invalid ? "" : selected ? "existing" : newDeal ? "new" : "none"
          }
          onChange={(value) =>
            choose(
              value === "existing" && first
                ? { id: first.id, company: preview?.company_id }
                : value === "new"
                  ? "new"
                  : "none",
            )
          }
        />
        {selected && (
          <Field label={t("lead.qualify.existingDealPicker")}>
            {(control) => (
              <Select
                {...control}
                value={selected.id}
                onChange={(id) => choose({ id, company: preview?.company_id })}
                options={candidates.map((deal) => ({
                  value: deal.id,
                  label: [
                    deal.name || t("lead.qualify.unnamedDeal"),
                    deal.amount_minor != null && deal.currency
                      ? formatMoney(deal.amount_minor, deal.currency, locale)
                      : "",
                    deal.id.slice(-6),
                  ]
                    .filter(Boolean)
                    .join(" · "),
                }))}
              />
            )}
          </Field>
        )}
        {more && (
          <Button variant="link" pending={fetching} onClick={loadMore}>
            {t("lead.qualify.moreDeals")}
          </Button>
        )}
      </section>
      {invalid && (
        <>
          <p role="status">{t("lead.qualify.dealUnavailable")}</p>
          <Button variant="link" onClick={() => choose("none")}>
            {t("lead.qualify.withoutDeal")}
          </Button>
        </>
      )}
    </>
  );
}

export function qualificationConfirmState(
  deal: ReturnType<typeof qualificationDealState>,
  preview: { isPending: boolean; isFetching: boolean },
  amount: { amountInvalid: boolean },
  // nameSettling: the deal would be named after a company name still being read.
  name: { nameSettling: boolean },
) {
  return {
    label: qualificationConfirmLabel(Boolean(deal.selectedDeal), deal.withDeal),
    disabled:
      preview.isPending ||
      preview.isFetching ||
      deal.invalidSelection ||
      (deal.withDeal && (amount.amountInvalid || name.nameSettling)) ||
      (Boolean(deal.selectedDeal) && preview.isFetching),
  };
}

function qualificationConfirmLabel(
  existing: boolean,
  create: boolean,
):
  | "lead.qualify.confirmExisting"
  | "lead.qualify.confirmWithDeal"
  | "lead.qualify.confirm" {
  if (existing) return "lead.qualify.confirmExisting";
  if (create) return "lead.qualify.confirmWithDeal";
  return "lead.qualify.confirm";
}

// dealBlock is the wire's deal request from the dialog's fields. The money
// pair holds from birth: an amount only travels with the currency, and no
// amount travels as no money at all.
export function dealBlock(input: {
  pipelineId: string | undefined;
  stageId: string | undefined;
  name: string;
  amountMinor: number | null;
  currency: string | undefined;
}): NonNullable<PromoteLeadRequest["deal"]> {
  const priced = input.amountMinor !== null && input.currency;
  return {
    pipeline_id: input.pipelineId ?? null,
    stage_id: input.stageId ?? null,
    name: input.name.trim() || null,
    amount_minor: priced ? input.amountMinor : null,
    currency: priced ? (input.currency ?? null) : null,
  };
}

// qualifyRefusals splits a failed promotion: a refusal of the amount stands
// under the field, and the foot of the dialog keeps everything else.
export function qualifyRefusals(
  error: unknown,
  t: (key: MessageKey) => string,
) {
  const faults = problemFieldErrorsOf(error);
  const amount = faults.find((fault) => fault.field === "amount_minor");
  return {
    amount: amount && fieldRefusalSentence(amount, t),
    foot:
      amount && faults.length === 1 ? undefined : problemMessageOf(error, t),
  };
}

// The sentence under the amount, and whether the box's own state holds the
// confirm back. Text the box refuses names its reason under the box itself.
export function amountRefusal(
  amount: { amountMinor: number | null; refused: boolean },
  server: string | undefined,
  t: ReturnType<typeof useT>,
) {
  if (amount.refused) {
    return { local: true, sentence: undefined };
  }
  if (amount.amountMinor !== null && amount.amountMinor < 0) {
    return { local: true, sentence: t("create.amountNegative") };
  }
  return { local: false, sentence: server };
}

export function openStagesOf(pipeline: Pipeline | undefined) {
  return [...(pipeline?.stages ?? [])]
    .filter((stage) => stage.semantic === "open")
    .sort((a, b) => a.position - b.position);
}
