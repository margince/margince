// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { Switch } from "../../design-system/switch";
import { useT } from "../../i18n";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { approvalKindLabel } from "./approvalkind";
import { ApprovalRow } from "./approvalrow";

export function ApprovalPreferencesCard() {
  const t = useT();
  const client = useQueryClient();
  const [reviewKind, setReviewKind] = useState<string>();
  const query = useQuery({
    queryKey: ["approval-preferences"],
    queryFn: async () => unwrap(await api.GET("/me/approval-preferences")),
  });
  const update = useMutation({
    scope: { id: "approval-preferences" },
    mutationFn: async (choice: { kind: string; muted: boolean }) =>
      unwrap(await api.PATCH("/me/approval-preferences", { body: choice })),
    onSuccess: async (data) => {
      client.setQueryData(["approval-preferences"], data);
      await Promise.all(
        ["autonomy", "approvals", "worklist", "brief", "magic", "digest"].map(
          (key) => client.invalidateQueries({ queryKey: [key] }),
        ),
      );
    },
  });
  return (
    <Panel title={t("approvalPreferences.title")}>
      <PanelBody>
        <PanelIntro>{t("approvalPreferences.explain")}</PanelIntro>
      </PanelBody>
      <QueryGate query={query} pendingLabel={t("approvalPreferences.title")}>
        {(result) => (
          <SettingList bleed="settings">
            {result.data.map((row) => (
              <SettingRow
                key={row.kind}
                label={approvalKindLabel(row.kind, t)}
                description={
                  <Button
                    variant="link"
                    onClick={() => setReviewKind(row.kind)}
                  >
                    {t("approvalPreferences.review")}
                  </Button>
                }
                control={
                  <Switch
                    label={approvalKindLabel(row.kind, t)}
                    labelHidden
                    checked={row.muted}
                    pending={update.isPending}
                    onChange={(muted) =>
                      update.mutate({ kind: row.kind, muted })
                    }
                  />
                }
              />
            ))}
          </SettingList>
        )}
      </QueryGate>
      {update.isError && (
        <PanelBody>
          <Callout tone="danger" title={t("approvalPreferences.title")}>
            {problemMessageOf(update.error, t)}
          </Callout>
        </PanelBody>
      )}
      {reviewKind && (
        <PreferenceReview
          key={reviewKind}
          kind={reviewKind}
          close={() => setReviewKind(undefined)}
        />
      )}
    </Panel>
  );
}

function PreferenceReview({
  kind,
  close,
}: Readonly<{ kind: string; close: () => void }>) {
  const t = useT();
  const [cursor, setCursor] = useState<string>();
  const query = useQuery({
    queryKey: ["approvals", "include-muted", kind, cursor],
    queryFn: async () =>
      unwrap(
        await api.GET("/approvals", {
          params: {
            query: {
              kind,
              include_muted: true,
              cursor,
              limit: 20,
            },
          },
        }),
      ),
  });
  return (
    <PanelBody>
      <PanelIntro>{t("approvalPreferences.reviewExplain")}</PanelIntro>
      <Button onClick={close}>{t("approvalPreferences.close")}</Button>
      <QueryGate
        query={query}
        pendingLabel={t("approvalPreferences.review")}
        empty={(result) => result.data.length === 0}
      >
        {(result) => (
          <>
            {result.data.map((approval) => (
              <ApprovalRow
                key={approval.id}
                approval={approval}
                decided={approval.status !== "pending"}
              />
            ))}
            {result.page?.has_more && result.page.next_cursor && (
              <Button
                onClick={() => setCursor(result.page?.next_cursor ?? undefined)}
              >
                {t("approvalPreferences.next")}
              </Button>
            )}
            {cursor && (
              <Button onClick={() => setCursor(undefined)}>
                {t("approvalPreferences.first")}
              </Button>
            )}
          </>
        )}
      </QueryGate>
    </PanelBody>
  );
}
