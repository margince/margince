import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type ComponentProps, useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { EntityRef } from "../../components/entityref";
import { LostReasonFields } from "../../components/lostreasonfields";
import { Button, Modal } from "../../design-system/atoms";
import { Heading } from "../../design-system/heading";
import { QueryGate } from "../../design-system/querystates";
import {
  type ResolveAnswer,
  ResolveSheet,
} from "../../design-system/resolvesheet";
import { formatDate } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import {
  choosableLossReasons,
  useLossReasons,
} from "../../lib/lossreasons.queries";
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { type ReviewActions, useReviewActions } from "../../lib/reviewactions";
import { CallCheckSheet } from "./inputcheck.call";
import { CheckFacts } from "./inputcheck.facts";

type InputCheck = components["schemas"]["InputCheck"];
type Props = Omit<ComponentProps<typeof ResolveSheet>, "onSubmit"> & {
  check?: InputCheck;
  // lostReasonCode rides only with a review answered by closing the deal lost.
  onSubmit: (
    answer: ResolveAnswer,
    dealId?: string,
    lostReasonCode?: string,
  ) => void;
};

// The answers a customer review takes, as the server accepts them
// (assurance.ReviewOutcomes). The sheet words them as a rep would: the
// customer is back, asked to wait, the value is right anyway, or it is lost.
const reviewOutcomes = new Set([
  "fixed_record",
  "remind_later",
  "value_correct",
  "not_relevant",
]);

export function InputCheckAnswerSheet(props: Readonly<Props>) {
  const actions = useReviewActions();
  if (props.check?.type === "forecast_call_moved") {
    return (
      <CallCheckSheet
        check={props.check}
        open={props.open}
        onClose={props.onClose}
        returnFocusTo={props.returnFocusTo}
        closeLabel={props.labels.cancel}
      />
    );
  }
  if (props.check?.type === "deal_viability") {
    return <ReviewAnswerSheet {...props} check={props.check} />;
  }
  if (props.check?.type !== "unlinked_signal") {
    return (
      <ResolveSheet
        {...props}
        context={props.check ? <CheckFacts check={props.check} /> : undefined}
      />
    );
  }
  if (!actions) {
    throw new Error("The app shell must supply review actions");
  }
  return <SignalCheckAnswerSheet {...props} actions={actions} />;
}

// A customer review answered by closing the deal lost asks why from the same
// picker as every other close-lost dialog. The sheet's own reason is the note.
function ReviewAnswerSheet(props: Readonly<Props & { check: InputCheck }>) {
  const t = useT();
  const [code, setCode] = useState("");
  const reasons = choosableLossReasons(useLossReasons(props.open).data);
  const closesLost = (outcome: string) => outcome === "not_relevant";
  return (
    <ResolveSheet
      {...props}
      labels={reviewLabels(t, props.labels)}
      asksExpiry={(outcome) => !closesLost(outcome)}
      context={<CheckFacts check={props.check} />}
      outcomeFields={(outcome) =>
        closesLost(outcome) && (
          <LostReasonFields reasons={reasons} code={code} onCode={setCode} />
        )
      }
      canSubmit={(outcome) => !closesLost(outcome) || code !== ""}
      onSubmit={(answer) =>
        props.onSubmit(
          answer,
          undefined,
          closesLost(answer.outcome) ? code : undefined,
        )
      }
    />
  );
}

function reviewLabels(
  t: ReturnType<typeof useT>,
  labels: Props["labels"],
): Props["labels"] {
  const words: Partial<Record<string, { label: string; description: string }>> =
    {
      fixed_record: {
        label: t("review.revive"),
        description: t("review.reviveHelp"),
      },
      remind_later: {
        label: t("review.wait"),
        description: t("review.waitHelp"),
      },
      not_relevant: {
        label: t("review.closeLost"),
        description: t("review.closeLostHelp"),
      },
    };
  return {
    ...labels,
    remindAt: t("review.waitUntil"),
    outcomes: labels.outcomes
      .filter((outcome) => reviewOutcomes.has(outcome.value))
      .map((outcome) => ({ ...outcome, ...words[outcome.value] })),
  };
}

function SignalCheckAnswerSheet({
  actions,
  check,
  onSubmit,
  labels,
  ...props
}: Readonly<Props & { actions: ReviewActions }>) {
  const {
    NewDealAction,
    CompanyDealPicker,
    CoveredDealSuggestion,
    OpenActivityDrawer,
  } = actions;
  const t = useT();
  const queryClient = useQueryClient();
  const headingId = useId();
  const mayCreate = useCanWrite("deal", "create");
  const { locale } = useLocale();
  const zone = useRecordZone();
  const [dealId, setDealId] = useState("");
  const [openedSource, setOpenedSource] = useState<string | null>(null);
  const mayLink = useCanWrite("deal", "update");
  const mayAnswer = useCanWrite("signal", "update");
  const isSignal = check?.type === "unlinked_signal";
  const companyId = check?.claim?.company_id;
  const suggestionId = check?.observed?.suggestion_id;
  const occurredAt = check?.observed?.occurred_at;
  const signalId = isSignal ? check?.subject_id : undefined;
  const signal = useQuery({
    queryKey: ["signal", signalId],
    enabled: props.open && Boolean(signalId),
    queryFn: async () =>
      unwrap(
        await api.GET("/signals/{id}", {
          params: { path: { id: signalId ?? "" } },
        }),
      ),
  });
  const signalLabels = {
    ...labels,
    outcomes: labels.outcomes
      .filter(
        (outcome) =>
          outcome.value === "not_relevant" ||
          outcome.value === "remind_later" ||
          (outcome.value === "fixed_record" && mayLink),
      )
      .map((outcome) =>
        outcome.value === "fixed_record"
          ? { ...outcome, label: t("dealSuggestion.linkExisting") }
          : outcome.value === "not_relevant"
            ? { ...outcome, label: t("review.signalNotRelevant") }
            : outcome,
      ),
  };
  if (isSignal && typeof suggestionId === "string" && (mayCreate || mayLink)) {
    return (
      <Modal
        intent="form"
        open={props.open}
        onClose={props.onClose}
        labelledBy={headingId}
        returnFocusTo={props.returnFocusTo}
      >
        <Heading id={headingId} size="large" className="modal-title">
          {labels.title}
        </Heading>
        <CoveredDealSuggestion
          companyId={typeof companyId === "string" ? companyId : undefined}
          suggestionId={suggestionId}
          open={props.open}
        />
        <div className="actions">
          <Button onClick={props.onClose}>{labels.cancel}</Button>
        </div>
      </Modal>
    );
  }
  if (isSignal && !mayAnswer) {
    return (
      <Modal
        intent="form"
        open={props.open}
        labelledBy={headingId}
        onClose={props.onClose}
        returnFocusTo={props.returnFocusTo}
      >
        <Heading size="large" id={headingId} className="modal-title">
          {labels.title}
        </Heading>
        <p>{t("common.permissionDenied")}</p>
        <div className="actions">
          <Button onClick={props.onClose}>{labels.cancel}</Button>
        </div>
      </Modal>
    );
  }
  return (
    <>
      <ResolveSheet
        {...props}
        labels={isSignal ? signalLabels : labels}
        canSubmit={(outcome) =>
          !isSignal ||
          (signal.isSuccess && (outcome !== "fixed_record" || dealId !== ""))
        }
        onSubmit={(answer) =>
          onSubmit(
            answer,
            isSignal && answer.outcome === "fixed_record" ? dealId : undefined,
          )
        }
        outcomeFields={(outcome) =>
          isSignal && (
            <>
              {check?.subject?.type === "company" && (
                <EntityRef
                  kind="company"
                  id={check.subject.id}
                  name={check.subject.label}
                />
              )}
              {check?.owner_id && (
                <p>
                  {t("dealSuggestion.field.owner")}:{" "}
                  <EntityRef kind="user" id={check.owner_id} />
                </p>
              )}
              {check?.observed?.buyer_verified === false && (
                <p className="sub">{t("review.uncertainBuyer")}</p>
              )}
              {typeof occurredAt === "string" && (
                <p className="sub">
                  {t("review.signalOccurred", {
                    date: formatDate(occurredAt, locale, zone),
                  })}
                </p>
              )}
              <QueryGate query={signal} pendingLabel={t("stat.evidence")}>
                {(source) => (
                  <>
                    <p>{source.summary}</p>
                    {source.evidence.map((evidence) => (
                      <div
                        key={`${evidence.source_type}:${evidence.source_id}:${evidence.snippet}`}
                      >
                        <p>{evidence.snippet}</p>
                        {evidence.source_type === "activity" &&
                          evidence.source_id && (
                            <Button
                              onClick={() =>
                                setOpenedSource(evidence.source_id ?? null)
                              }
                            >
                              {t("co.signals.openSource")}
                            </Button>
                          )}
                      </div>
                    ))}
                  </>
                )}
              </QueryGate>
              {check?.observed?.historical === true && (
                <p className="sub">{t("review.historicalSignal")}</p>
              )}
              {outcome === "fixed_record" && typeof companyId === "string" && (
                <>
                  {mayCreate && (
                    <NewDealAction
                      companyId={companyId}
                      companyName={check?.subject?.label ?? ""}
                      onCreated={(deal) => {
                        setDealId(deal.id);
                        queryClient.invalidateQueries({ queryKey: ["deals"] });
                      }}
                    />
                  )}
                  <CompanyDealPicker
                    companyId={companyId}
                    value={dealId}
                    onChange={setDealId}
                  />
                </>
              )}
            </>
          )
        }
      />
      <OpenActivityDrawer
        activityId={openedSource}
        zone={zone}
        onClose={() => setOpenedSource(null)}
      />
    </>
  );
}
