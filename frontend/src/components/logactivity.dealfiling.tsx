import { skipToken, useQuery } from "@tanstack/react-query";
import { type ReactNode, useCallback, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, Field } from "../design-system/atoms";
import { QueryGate } from "../design-system/querystates";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../design-system/recordpicker";
import { useT } from "../i18n";
import type { ActivityDealSelection } from "../lib/activitybody";
import type { EntityKind } from "../lib/entity";
import { unwrap } from "../lib/problem";
import { EntityRef } from "./entityref";

type DealChoice =
  | Readonly<{ kind: "none" }>
  | Readonly<{ kind: "deal"; candidate: RecordPickerCandidate }>;

export const activityDealFilingKey = (
  type: string | undefined,
  id: string | undefined,
) => ["activity-deal-filing", type, id] as const;

export function useLogDealFiling(
  entityType: EntityKind | undefined,
  entityId: string | undefined,
  active: boolean,
): Readonly<{
  field: ReactNode;
  blocked: boolean;
  reason: string | undefined;
  request: ActivityDealSelection | undefined;
  clear: () => void;
}> {
  const t = useT();
  const ref =
    active && entityId && (entityType === "contact" || entityType === "company")
      ? { record_type: entityType, record_id: entityId }
      : undefined;
  const read = useQuery({
    queryKey: activityDealFilingKey(ref?.record_type, ref?.record_id),
    queryFn: ref
      ? async () =>
          unwrap(
            await api.GET("/activities/deal-filing", {
              params: { query: ref },
            }),
          )
      : skipToken,
    staleTime: 0,
  });
  const companyId = read.data?.company_id;
  const contextKey = `${ref?.record_type}:${ref?.record_id}:${companyId}`;
  const [choice, setChoice] = useState<{
    contextKey: string;
    value: DealChoice;
  } | null>(null);
  const held = choice?.contextKey === contextKey ? choice.value : null;
  const pick = (value: DealChoice) => setChoice({ contextKey, value });
  const search = useCallback(
    async (q: string) => {
      if (!companyId) return [];
      const page = unwrap(
        await api.GET("/deals", {
          params: {
            query: { q, company_id: companyId, status: "open", limit: 20 },
          },
        }),
      );
      return page.data.map((deal) => ({ id: deal.id, name: deal.name }));
    },
    [companyId],
  );
  const request =
    ref && read.isSuccess ? selectionFor(ref, read.data, held) : undefined;
  const reason = filingBlockReason(Boolean(ref), read, held !== null, t);
  return {
    request,
    reason,
    blocked: reason !== undefined,
    clear: () => setChoice(null),
    field: ref ? (
      <QueryGate
        query={read}
        empty={() => false}
        pendingLabel={t("log.dealLoading")}
      >
        {(options) =>
          options.choice_required || held ? (
            <div className="form-stack">
              <p className="t-caption">{t("log.dealChoice")}</p>
              <Field label={t("log.dealFiling")}>
                {(control) => (
                  <RecordPicker
                    {...control}
                    searchTargets={search}
                    searchOnFocus
                    onSearchChange={() => setChoice(null)}
                    selected={held?.kind === "deal" ? held.candidate : null}
                    onPick={(candidate) => pick({ kind: "deal", candidate })}
                  />
                )}
              </Field>
              <Button
                variant="ghost"
                aria-pressed={held?.kind === "none"}
                onClick={() => pick({ kind: "none" })}
              >
                {t("log.dealNone")}
              </Button>
            </div>
          ) : options.suggested_deal_id ? (
            <p className="t-caption">
              {t("log.dealAutomatic")}{" "}
              <EntityRef kind="deal" id={options.suggested_deal_id} />
            </p>
          ) : null
        }
      </QueryGate>
    ) : null,
  };
}

function selectionFor(
  context: Pick<
    components["schemas"]["ActivityDealFiling"],
    "record_type" | "record_id"
  >,
  options: components["schemas"]["ActivityDealFilingOptions"],
  choice: DealChoice | null,
): ActivityDealSelection {
  const dealId = choice?.kind === "deal" ? choice.candidate.id : undefined;
  return {
    context: {
      ...context,
      skip: choice?.kind === "none",
      deal_id:
        choice?.kind === "none"
          ? undefined
          : (dealId ?? options.suggested_deal_id),
    },
    dealId,
  };
}

function filingBlockReason(
  active: boolean,
  read: {
    isError: boolean;
    isSuccess: boolean;
    isFetching: boolean;
    data?: components["schemas"]["ActivityDealFilingOptions"];
  },
  chosen: boolean,
  t: ReturnType<typeof useT>,
): string | undefined {
  if (!active) return undefined;
  if (read.isError) return t("log.dealUnavailable");
  if (!read.isSuccess || read.isFetching) return t("log.dealLoading");
  if (read.data?.choice_required && !chosen) return t("log.dealChoice");
  return undefined;
}
