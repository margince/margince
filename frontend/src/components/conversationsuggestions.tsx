// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Panel, PanelBody, PanelRow } from "../design-system/panel";
import { useT } from "../i18n";
import { useCan, useCanWrite } from "../lib/capability";
import { nameFromEmail } from "../lib/namefromemail";
import {
  ProblemError,
  problemCodeOf,
  problemExistingId,
  unwrap,
} from "../lib/problem";
import { leadHeldNoticeFor } from "./leadheldnotice";

type Suggestion = components["schemas"]["ConversationSuggestion"];
type Suggestions = components["schemas"]["ConversationSuggestionListResponse"];
type RecordAnchor = Readonly<{ type: "contact" | "lead"; id: string }>;
type AddContact = Readonly<{
  suggestion: Suggestion;
  idempotencyKey: string;
  convertLeadIds?: string[];
}>;

export function ConversationSuggestionsCard({
  record,
}: Readonly<{ record: RecordAnchor }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const canRead = useCan("activity", "read");
  const canCreate = useCanWrite("contact", "create");
  const key = ["conversation-suggestions", record.type, record.id];
  const query = useQuery({
    queryKey: key,
    enabled: canRead,
    queryFn: async ({ signal }) => {
      const result = unwrap(
        record.type === "contact"
          ? await api.GET("/contacts/{id}/conversation-suggestions", {
              params: { path: { id: record.id } },
              signal,
            })
          : await api.GET("/leads/{id}/conversation-suggestions", {
              params: { path: { id: record.id } },
              signal,
            }),
        t,
      );
      if (!Array.isArray(result?.data) || !result.data.every(validSuggestion)) {
        throw new Error("Malformed conversation suggestions response");
      }
      return result;
    },
  });
  const remove = (suggestion: Suggestion) => {
    queryClient.setQueryData<Suggestions>(
      key,
      (old) =>
        old && {
          ...old,
          data: old.data.filter((row) => row.address !== suggestion.address),
        },
    );
    void queryClient.invalidateQueries({
      queryKey: ["conversation-suggestions"],
    });
    void queryClient.invalidateQueries({ queryKey: ["contacts"] });
  };
  const add = useMutation({
    mutationFn: async ({
      suggestion,
      idempotencyKey,
      convertLeadIds,
    }: AddContact) =>
      unwrap(
        await api.POST("/contacts", {
          params: { header: { "Idempotency-Key": idempotencyKey } },
          body: {
            full_name:
              suggestion.display_name || nameFromEmail(suggestion.address),
            emails: [
              {
                email: suggestion.address,
                email_type: "work",
                is_primary: true,
                position: 0,
              },
            ],
            source: "manual",
            convert_lead_ids: convertLeadIds,
          },
        }),
        t,
      ),
    onSuccess: (_contact, variables) => remove(variables.suggestion),
    onError: (error, variables) => {
      if (
        problemCodeOf(error) === "duplicate_email" &&
        error instanceof ProblemError &&
        problemExistingId(error.problem)
      )
        remove(variables.suggestion);
    },
  });
  const notice = leadHeldNoticeFor(
    add.error,
    (extra) => {
      const ids = extra.convert_lead_ids;
      if (
        !add.variables ||
        !Array.isArray(ids) ||
        !ids.every((id): id is string => typeof id === "string")
      )
        return;
      add.mutate({
        suggestion: add.variables.suggestion,
        idempotencyKey: crypto.randomUUID(),
        convertLeadIds: ids,
      });
    },
    add.isPending,
  );
  if (
    !canRead ||
    query.isPending ||
    (!query.isError && !query.data?.data.length)
  )
    return null;
  return (
    <Panel title={t("conversationSuggestions.title")}>
      {query.isError ? (
        <PanelBody>
          <ErrorLine
            error={query.error}
            actions={
              <Button onClick={() => void query.refetch()}>
                {t("common.retry")}
              </Button>
            }
          />
        </PanelBody>
      ) : (
        query.data?.data.map((suggestion) => (
          <PanelRow key={suggestion.address}>
            <div>
              <div>
                {suggestion.display_name || nameFromEmail(suggestion.address)}
              </div>
              <div className="t-caption">{suggestion.address}</div>
            </div>
            {canCreate && (
              <Button
                pending={
                  add.isPending &&
                  add.variables?.suggestion.address === suggestion.address
                }
                disabled={add.isPending}
                onClick={() =>
                  add.mutate({
                    suggestion,
                    idempotencyKey: crypto.randomUUID(),
                  })
                }
              >
                {t("conversationSuggestions.add")}
              </Button>
            )}
          </PanelRow>
        ))
      )}
      {query.data?.scan_limited && (
        <PanelBody>
          <p className="t-caption">{t("conversationSuggestions.limited")}</p>
        </PanelBody>
      )}
      {(add.isError || notice) && (
        <PanelBody>{notice || <ErrorLine error={add.error} />}</PanelBody>
      )}
    </Panel>
  );
}

function validSuggestion(value: unknown): value is Suggestion {
  return (
    typeof value === "object" &&
    value !== null &&
    "address" in value &&
    typeof value.address === "string" &&
    value.address.length > 0 &&
    "activity_id" in value &&
    typeof value.activity_id === "string" &&
    (!("display_name" in value) ||
      value.display_name == null ||
      typeof value.display_name === "string")
  );
}
