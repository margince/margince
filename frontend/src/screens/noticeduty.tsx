// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The privacy-notice duty, where a reader meets it: on the Focus card's detail
// pane. It says what the duty is and offers the three ways to discharge it.
//
// The card used to say "Disclosure owed to contact", a deadline, and nothing a
// reader could do: the actions lived on the contact's consent panel and in
// Settings → Privacy, and nothing named the rule. A duty nobody can act on from
// the place it is raised is a duty nobody discharges.

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import { useCan } from "../app/capability";
import { Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Panel, PanelBody } from "../design-system/panel";
import { useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";
import { ExcuseModal, type ExcuseState } from "./noticeexcuse";

// Every read that could still show the duty: the Home brief, the worklist the
// Focus list draws from, the settings queue and the contact's consent panel.
const DUTY_READS = [
  ["brief"],
  ["worklist"],
  ["notice-cases"],
  ["contact-consent"],
];

export function NoticeDuty({
  caseId,
  contactId,
}: Readonly<{ caseId: string; contactId: string }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const mayWrite = useCan("contact", "update");
  const [excusing, setExcusing] = useState(false);
  const [done, setDone] = useState<string | null>(null);
  const settle = (sentence: string) => {
    setDone(sentence);
    for (const key of DUTY_READS) {
      void queryClient.invalidateQueries({ queryKey: key });
    }
  };

  const send = useMutation({
    mutationFn: async (route: "privacy-notice" | "confirm-request") => {
      const { data, error } =
        route === "privacy-notice"
          ? await api.POST("/contacts/{id}/consent/privacy-notice", {
              params: { path: { id: contactId } },
            })
          : await api.POST("/contacts/{id}/consent/confirm-request", {
              params: { path: { id: contactId } },
            });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (issued) =>
      settle(
        issued.queued
          ? t("noticeDuty.sent", { address: issued.delivered_to })
          : t("noticeDuty.notSent", { address: issued.delivered_to }),
      ),
  });

  const excuse = useMutation({
    mutationFn: async (vars: { state: ExcuseState; note: string }) => {
      const { error } = await api.POST("/privacy/notice-cases/{id}/excuse", {
        params: { path: { id: caseId } },
        body: { state: vars.state, resolution_note: vars.note },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      setExcusing(false);
      settle(t("noticeDuty.ended"));
    },
  });

  return (
    <Panel title={t("noticeDuty.title")}>
      <PanelBody>
        <p>{t("noticeDuty.what")}</p>
        <p className="t-caption">{t("noticeDuty.how")}</p>
        {done ? (
          <p role="status">{done}</p>
        ) : (
          mayWrite && (
            <div className="notice-duty-actions">
              <Button
                variant="primary"
                disabled={send.isPending}
                onClick={() => send.mutate("privacy-notice")}
              >
                {t("noticeDuty.sendNotice")}
              </Button>
              <Button
                disabled={send.isPending}
                onClick={() => send.mutate("confirm-request")}
              >
                {t("noticeDuty.askConfirm")}
              </Button>
              <Button onClick={() => setExcusing(true)}>
                {t("noticeDuty.end")}
              </Button>
            </div>
          )
        )}
        {send.isError && <ErrorLine error={send.error} />}
        <ExcuseModal
          open={excusing}
          onClose={() => setExcusing(false)}
          onConfirm={(state, note) => excuse.mutate({ state, note })}
          pending={excuse.isPending}
          error={excuse.isError ? problemMessageOf(excuse.error, t) : null}
        />
      </PanelBody>
    </Panel>
  );
}
