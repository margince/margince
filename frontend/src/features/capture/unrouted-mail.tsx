// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { Panel, PanelBody, PanelIntro } from "../../design-system/panel";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { undoAction, useToast } from "../../design-system/toast";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { problemMessageOf, throwProblem, unwrap } from "../../lib/problem";
import { DomainQuestionCallout, domainQuestionOf } from "./domainquestion";

// Mail in the reader's own mailbox that was sent to addresses that are not
// theirs. Nothing judges it until the reader says, per address, whether they
// receive it. "I receive this address" is the ordinary own-address claim, so
// capture then checks that mail like the rest. "Not mine" keeps it hidden and
// stops the count. The list is the reader's own mailbox and nobody else's.

type UnroutedMail = components["schemas"]["UnroutedMailResponse"];

const KEY = ["capture-unrouted-mail"];

function useUnroutedMail() {
  return useQuery({
    queryKey: KEY,
    queryFn: async (): Promise<UnroutedMail> => {
      const { data, error, response } = await api.GET("/capture/unrouted-mail");
      if (error || !response.ok) throwProblem(error);
      return data;
    },
  });
}

function useRefresh() {
  const queryClient = useQueryClient();
  const owners = ["capture-owner-identities"];
  return () => {
    queryClient.invalidateQueries({ queryKey: KEY });
    queryClient.invalidateQueries({ queryKey: owners });
  };
}

function useReceive() {
  const t = useT();
  const toast = useToast();
  const refresh = useRefresh();
  const withdraw = useMutation({
    mutationFn: async (id: string) =>
      unwrap(
        await api.DELETE("/capture/owner-identities/{id}", {
          params: { path: { id } },
        }),
      ),
    onError: (error) =>
      toast.show(problemMessageOf(error, t), { tone: "danger", sticky: true }),
    onSuccess: () => {
      refresh();
      toast.show(t("unroutedMail.removedClaim"));
    },
  });
  return useMutation({
    mutationFn: async (claim: { value: string; confirm?: boolean }) =>
      unwrap(
        await api.POST("/capture/owner-identities", {
          body: { kind: "address", ...claim },
        }),
      ),
    // The address-on-file question is asked in place, under the list.
    onError: (error) => {
      if (domainQuestionOf(error) === null) {
        toast.show(problemMessageOf(error, t), {
          tone: "danger",
          sticky: true,
        });
      }
    },
    onSuccess: (identity) => {
      refresh();
      toast.show(t("unroutedMail.received"), {
        action: undoAction(t("common.undo"), () =>
          withdraw.mutate(identity.id),
        ),
      });
    },
  });
}

function useRefusal() {
  const t = useT();
  const toast = useToast();
  const refresh = useRefresh();
  const restore = useMutation({
    mutationFn: async (address: string) =>
      unwrap(
        await api.DELETE("/capture/unrouted-mail/{address}/refusal", {
          params: { path: { address } },
        }),
      ),
    onError: (error) =>
      toast.show(problemMessageOf(error, t), { tone: "danger", sticky: true }),
    onSuccess: () => {
      refresh();
      toast.show(t("unroutedMail.restored"));
    },
  });
  return useMutation({
    mutationFn: async (address: string) =>
      unwrap(
        await api.PUT("/capture/unrouted-mail/{address}/refusal", {
          params: { path: { address } },
        }),
      ),
    onError: (error) =>
      toast.show(problemMessageOf(error, t), { tone: "danger", sticky: true }),
    onSuccess: (_, address) => {
      refresh();
      toast.show(t("unroutedMail.refused"), {
        action: undoAction(t("common.undo"), () => restore.mutate(address)),
      });
    },
  });
}

/** Renders nothing while the reader's mailbox holds no mail for other addresses. */
export function UnroutedMailCard() {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const query = useUnroutedMail();
  const receive = useReceive();
  const refuse = useRefusal();
  const mail = query.data;
  if (!mail || mail.data.length === 0) return null;
  const busy = receive.isPending || refuse.isPending;
  return (
    <Panel title={t("unroutedMail.title")}>
      <PanelBody>
        <PanelIntro>{t("unroutedMail.sub")}</PanelIntro>
        <p className="t-caption" data-testid="unrouted-mail-summary">
          {plural("unroutedMail.summary", mail.messages, {
            count: formatNumber(mail.messages, locale),
          })}
        </p>
      </PanelBody>
      <SettingList bleed="settings" testId="unrouted-mail-list">
        {mail.data.map((row) => (
          <SettingRow
            key={row.address}
            label={row.address}
            description={plural("unroutedMail.count", row.messages, {
              count: formatNumber(row.messages, locale),
            })}
            control={
              <span className="cell-actions">
                <Button
                  disabled={busy}
                  aria-label={t("unroutedMail.receiveLabel", {
                    value: row.address,
                  })}
                  onClick={() => receive.mutate({ value: row.address })}
                >
                  {t("unroutedMail.receive")}
                </Button>
                <Button
                  variant="ghost"
                  disabled={busy}
                  aria-label={t("unroutedMail.notMineLabel", {
                    value: row.address,
                  })}
                  onClick={() => refuse.mutate(row.address)}
                >
                  {t("unroutedMail.notMine")}
                </Button>
              </span>
            }
          />
        ))}
      </SettingList>
      {receive.isError && receive.variables && (
        <PanelBody>
          <DomainQuestionCallout
            error={receive.error}
            value={receive.variables.value}
            scope="ownAddress"
            pending={receive.isPending}
            t={t}
            onConfirm={() =>
              receive.mutate({ value: receive.variables.value, confirm: true })
            }
          />
        </PanelBody>
      )}
      {mail.more && (
        <PanelBody>
          <p className="t-caption" data-testid="unrouted-mail-more">
            {t("unroutedMail.more")}
          </p>
        </PanelBody>
      )}
    </Panel>
  );
}
