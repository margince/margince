import { useQuery, useQueryClient } from "@tanstack/react-query";
import { type ComponentProps, useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { CompanyDealPicker } from "../../components/companydealpicker";
import { EntityRef } from "../../components/entityref";
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
import { unwrap } from "../../lib/problem";
import { useRecordZone } from "../../lib/recordzone";
import { NewDealAction } from "../companies/companyactions";
import { DealSuggestionCard } from "../deals/dealsuggestion";
import { useDealSuggestions } from "../deals/dealsuggestions.queries";
import { OpenActivityDrawer } from "../voice-knowledge/activitydrawer";

type InputCheck = components["schemas"]["InputCheck"];
type Props = Omit<ComponentProps<typeof ResolveSheet>, "onSubmit"> & {
  check?: InputCheck;
  onSubmit: (answer: ResolveAnswer, dealId?: string) => void;
};

export function InputCheckAnswerSheet({
  check,
  onSubmit,
  labels,
  ...props
}: Readonly<Props>) {
  const t = useT();
  const queryClient = useQueryClient();
  const headingId = useId();
  const mayCreate = useCanWrite("deal", "create");
  const { locale } = useLocale();
  const zone = useRecordZone();
  const [dealId, setDealId] = useState("");
  const [openedSource, setOpenedSource] = useState<string | null>(null);
  const mayLink = useCanWrite("deal", "update");
  const isSignal = check?.type === "unlinked_signal";
  const companyId = check?.claim?.company_id;
  const suggestionId = check?.observed?.suggestion_id;
  const suggestions = useDealSuggestions(
    { company_id: typeof companyId === "string" ? companyId : undefined },
    props.open &&
      typeof suggestionId === "string" &&
      typeof companyId === "string",
  );
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
        <Heading id={headingId} size="large">
          {labels.title}
        </Heading>
        <QueryGate query={suggestions} pendingLabel={t("stat.evidence")}>
          {(found) => {
            const suggestion = found.find((item) => item.id === suggestionId);
            return suggestion ? (
              <DealSuggestionCard suggestion={suggestion} />
            ) : (
              <p>{t("dealSuggestion.changed")}</p>
            );
          }}
        </QueryGate>
        <Button onClick={props.onClose}>{labels.cancel}</Button>
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
