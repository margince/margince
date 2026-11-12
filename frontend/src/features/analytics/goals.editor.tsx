// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useRef, useState } from "react";
import { Badge, Button, Field } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { ErrorLine } from "../../design-system/errorline";
import { Checkbox, Textarea } from "../../design-system/field";
import { MoneyInput } from "../../design-system/moneyinput";
import {
  formatAnalyticsMoney,
  formatNumber,
  identifierNumber,
  monthName,
} from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { problemMessageOf } from "../../lib/problem";
import { GoalBars } from "./goals.bars";
import { type GoalProposal, GoalProposalStep } from "./goals.proposal";
import {
  equalMonths,
  monthsTotal,
  type SalesGoalPlan,
  useSalesGoalWrites,
} from "./progress.queries";

type Months = (number | null)[];

// The months a manager starts from: the shared draft, else what is live.
function startingMonths(plan: SalesGoalPlan): Months {
  return [...(plan.draft?.months ?? plan.live_months)];
}

// One key per body sent, so a retry of the same save replays and an edited
// save is a new request.
function useRequestKey() {
  const last = useRef<{ body: string; key: string } | null>(null);
  return (body: unknown) => {
    const text = JSON.stringify(body);
    if (last.current?.body !== text)
      last.current = { body: text, key: globalThis.crypto.randomUUID() };
    return last.current.key;
  };
}

/**
 * A manager's editor for one team member's twelve months, from draft to
 * publication. `onCancel` goes back to the published plan it was opened from.
 */
export function GoalEditor({
  plan,
  onCancel,
}: Readonly<{ plan: SalesGoalPlan; onCancel?: () => void }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const writes = useSalesGoalWrites();
  const requestKey = useRequestKey();
  const [months, setMonths] = useState<Months>(() => startingMonths(plan));
  const [yearly, setYearly] = useState<number | null>(
    () => plan.draft?.declared_total_minor ?? monthsTotal(startingMonths(plan)),
  );
  const [reason, setReason] = useState(plan.draft?.reason ?? "");
  // Months taken from a proposal stay marked as one through every later save.
  const [proposed, setProposed] = useState(plan.draft?.origin === "ai");
  const [confirming, setConfirming] = useState<"publish" | "withdraw" | null>(
    null,
  );
  const total = monthsTotal(months);
  const path = { owner: plan.owner_id, year: plan.year, version: plan.version };
  const money = (minor: number) =>
    formatAnalyticsMoney(minor, plan.currency, locale);
  const failed = [writes.save, writes.discard].find(
    (write) => write.isError,
  )?.error;
  const draftBody = {
    months,
    ...(yearly === null ? {} : { declared_total_minor: yearly }),
    reason: reason.trim(),
    origin: proposed ? ("ai" as const) : ("manual" as const),
  };
  // Publishing sends the saved draft, so the yearly goal and the reason on
  // screen must be saved too.
  const unchangedDraft =
    plan.draft !== undefined &&
    (plan.draft.origin === "ai") === proposed &&
    plan.draft.reason.trim() === reason.trim() &&
    JSON.stringify(plan.draft.months) === JSON.stringify(months) &&
    (plan.draft.declared_total_minor ?? monthsTotal(plan.draft.months)) ===
      yearly;
  return (
    <div className="goal-editor">
      {plan.state === "no_goal" && (
        <p className="t-caption goal-note">{t("goals.start")}</p>
      )}
      <YearlyGoal
        plan={plan}
        yearly={yearly}
        proposed={proposed}
        onYearly={setYearly}
        onSpread={setMonths}
        onProposal={(proposal) => {
          setMonths(proposal.months);
          setYearly(proposal.total_minor);
          setReason((written) =>
            written.trim() === "" ? proposal.reason : written,
          );
          setProposed(true);
        }}
      />
      <GoalBars months={months} />
      <div className="goal-editor-months">
        {months.map((month, index) => (
          <Field
            key={monthName(index + 1, "en")}
            label={monthName(index + 1, locale)}
          >
            {(field) => (
              <MoneyInput
                {...field}
                valueMinor={month ?? 0}
                blankWhenZero={month === null}
                currency={plan.currency}
                display={money}
                onChangeMinor={(next) =>
                  setMonths((current) =>
                    current.map((each, at) => (at === index ? next : each)),
                  )
                }
                onClear={() =>
                  setMonths((current) =>
                    current.map((each, at) => (at === index ? null : each)),
                  )
                }
              />
            )}
          </Field>
        ))}
      </div>
      <p className="goal-editor-sum t-caption">
        {total === null
          ? plural(
              "goals.monthsMissing",
              months.filter((month) => month === null).length,
              {
                count: formatNumber(
                  months.filter((month) => month === null).length,
                  locale,
                ),
              },
            )
          : t("goals.monthsSum", { total: money(total) })}
      </p>
      {total !== null && yearly !== null && total !== yearly && (
        <Callout
          tone="warning"
          title={t("goals.sumDiffers", {
            yearly: money(yearly),
            total: money(total),
          })}
        />
      )}
      <Field
        label={t("goals.reason")}
        hint={`${t("goals.reasonHint")} ${t("goals.sellerReads", { name: plan.owner_name })}`}
      >
        {(field) => (
          <Textarea
            {...field}
            grow
            value={reason}
            maxLength={1000}
            onChange={(event) => setReason(event.target.value)}
          />
        )}
      </Field>
      {failed && <ErrorLine error={failed} />}
      <div className="goal-actions">
        {onCancel && (
          <Button variant="link" onClick={onCancel}>
            {t("reporting.cancel")}
          </Button>
        )}
        {plan.published && (
          <Button variant="ghost" onClick={() => setConfirming("withdraw")}>
            {t("goals.withdraw")}
          </Button>
        )}
        {plan.draft && (
          <Button
            variant="ghost"
            pending={writes.discard.isPending}
            onClick={() => writes.discard.mutate(path)}
          >
            {t("goals.discard")}
          </Button>
        )}
        <Button
          variant="ghost"
          pending={writes.save.isPending}
          reason={reason.trim() === "" ? t("goals.reasonNeeded") : undefined}
          onClick={() =>
            writes.save.mutate({
              ...path,
              body: draftBody,
              key: requestKey(draftBody),
            })
          }
        >
          {t("goals.saveDraft")}
        </Button>
        <Button
          variant="primary"
          reason={publishRefusal(plan, total, unchangedDraft, t)}
          onClick={() => setConfirming("publish")}
        >
          {t("goals.publish")}
        </Button>
      </div>
      {confirming === "publish" && total !== null && (
        <PublishConfirm
          plan={plan}
          total={money(total)}
          onClose={() => setConfirming(null)}
        />
      )}
      {confirming === "withdraw" && (
        <WithdrawConfirm plan={plan} onClose={() => setConfirming(null)} />
      )}
    </div>
  );
}

// The year in one figure, and the two ways to turn it into twelve months:
// spread it equally, or take a proposal read from the sales won so far.
function YearlyGoal({
  plan,
  yearly,
  proposed,
  onYearly,
  onSpread,
  onProposal,
}: Readonly<{
  plan: SalesGoalPlan;
  yearly: number | null;
  proposed: boolean;
  onYearly: (minor: number | null) => void;
  onSpread: (months: Months) => void;
  onProposal: (proposal: GoalProposal) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  return (
    <>
      <div className="goal-editor-yearly">
        <Field label={t("goals.yearly", { currency: plan.currency })}>
          {(field) => (
            <MoneyInput
              {...field}
              valueMinor={yearly ?? 0}
              blankWhenZero={yearly === null}
              currency={plan.currency}
              display={(minor) =>
                formatAnalyticsMoney(minor, plan.currency, locale)
              }
              onChangeMinor={onYearly}
              onClear={() => onYearly(null)}
            />
          )}
        </Field>
        <Button
          variant="ghost"
          disabled={yearly === null || yearly <= 0}
          onClick={() => yearly !== null && onSpread(equalMonths(yearly))}
        >
          {t("goals.spread")}
        </Button>
        <GoalProposalStep plan={plan} yearly={yearly} onApply={onProposal} />
      </div>
      {proposed && (
        <p className="goal-note">
          <Badge tone="ai">{t("goals.proposedByAi")}</Badge>
        </p>
      )}
    </>
  );
}

// Publication takes the saved draft, so the months on screen must be it.
function publishRefusal(
  plan: SalesGoalPlan,
  total: number | null,
  unchangedDraft: boolean,
  t: ReturnType<typeof useT>,
): string | undefined {
  if (!plan.draft || !unchangedDraft) return t("goals.saveFirst");
  if (total === null) return t("goals.completeFirst");
  if (total <= 0) return t("goals.positiveFirst");
  if (
    plan.draft.declared_total_minor != null &&
    plan.draft.declared_total_minor !== total
  )
    return t("goals.matchFirst");
  return undefined;
}

// The draft already carries its reason, so publishing shows it and asks
// only for the agreement.
function PublishConfirm({
  plan,
  total,
  onClose,
}: Readonly<{ plan: SalesGoalPlan; total: string; onClose: () => void }>) {
  const t = useT();
  const { publish } = useSalesGoalWrites();
  const requestKey = useRequestKey();
  const [agreed, setAgreed] = useState(false);
  const body = { reason: plan.draft?.reason.trim() ?? "" };
  return (
    <ConfirmModal
      open
      onClose={onClose}
      title={t("goals.publishTitle", {
        name: plan.owner_name,
        year: identifierNumber(plan.year),
      })}
      confirmLabel={t("goals.publish")}
      confirmReason={agreed ? undefined : t("goals.agreeFirst")}
      pending={publish.isPending}
      error={publish.error ? problemMessageOf(publish.error, t) : null}
      onConfirm={() =>
        publish.mutate(
          {
            owner: plan.owner_id,
            year: plan.year,
            version: plan.version,
            reason: body.reason,
            key: requestKey(body),
          },
          { onSuccess: onClose },
        )
      }
    >
      <p>{t("goals.publishBody", { total })}</p>
      <p className="t-caption">
        {t("goals.publishReason", { reason: body.reason })}
      </p>
      <Checkbox
        label={t("goals.agreed")}
        checked={agreed}
        onChange={(event) => setAgreed(event.target.checked)}
      />
    </ConfirmModal>
  );
}

/** Asks for the reason a published plan is withdrawn, then withdraws it. */
export function WithdrawConfirm({
  plan,
  onClose,
}: Readonly<{ plan: SalesGoalPlan; onClose: () => void }>) {
  const t = useT();
  const { withdraw } = useSalesGoalWrites();
  const [reason, setReason] = useState("");
  return (
    <ConfirmModal
      open
      onClose={onClose}
      title={t("goals.withdrawTitle", {
        name: plan.owner_name,
        year: identifierNumber(plan.year),
      })}
      confirmLabel={t("goals.withdraw")}
      confirmVariant="danger"
      confirmReason={reason.trim() === "" ? t("goals.reasonNeeded") : undefined}
      pending={withdraw.isPending}
      error={withdraw.error ? problemMessageOf(withdraw.error, t) : null}
      onConfirm={() =>
        withdraw.mutate(
          {
            owner: plan.owner_id,
            year: plan.year,
            version: plan.version,
            reason: reason.trim(),
          },
          { onSuccess: onClose },
        )
      }
    >
      <p>{t("goals.withdrawBody")}</p>
      <Field label={t("goals.reason")}>
        {(field) => (
          <Textarea
            {...field}
            grow
            value={reason}
            maxLength={1000}
            onChange={(event) => setReason(event.target.value)}
          />
        )}
      </Field>
    </ConfirmModal>
  );
}
