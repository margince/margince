import type { components } from "../../api/schema";
import { Field } from "../../design-system/atoms";
import { Button } from "../../design-system/button";
import { type Choice, ChoiceList } from "../../design-system/choicelist";
import { Select } from "../../design-system/select";
import { formatMoney } from "../../format/format";
import { useLocale, useT } from "../../i18n";

type Deal = components["schemas"]["Deal"];
type Preview = components["schemas"]["PromoteLeadPreview"];
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
  amount: { amountInvalid: boolean; amountWaitsForCurrency: boolean },
) {
  return {
    label: qualificationConfirmLabel(Boolean(deal.selectedDeal), deal.withDeal),
    disabled:
      preview.isPending ||
      preview.isFetching ||
      deal.invalidSelection ||
      (deal.withDeal &&
        (amount.amountInvalid || amount.amountWaitsForCurrency)) ||
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
