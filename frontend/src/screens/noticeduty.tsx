// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The privacy-notice duty, where a reader meets it: on the Focus card's detail
// pane. It says what the duty is and offers the three ways to discharge it.
//
// The card used to say "Disclosure owed to contact", a deadline, and nothing a
// reader could do: the actions lived on the contact's consent panel and in
// Settings → Privacy, and nothing named the rule. A duty nobody can act on from
// the place it is raised is a duty nobody discharges.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import { useCan } from "../app/capability";
import { Button } from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { type Fact, FactList } from "../design-system/factlist";
import { Panel, PanelBody } from "../design-system/panel";
import { formatDate } from "../format/format";
import { viewerZone } from "../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";
import { EntityRef } from "./entityref";
import {
  acquisitionKindLabel,
  acquisitionRecorder,
  noticeRuleHint,
  noticeRuleLabel,
} from "./noticeacquisition";
import type { NoticeCase } from "./noticecases.logic";
import { ExcuseModal, type ExcuseState } from "./noticeexcuse";
import type { WorklistItem } from "./worklist.queries";

// Every read that could still show the duty: the Home brief, the worklist the
// Focus list draws from, the settings queue and the contact's consent panel.
const DUTY_READS = [
  ["brief"],
  ["worklist"],
  ["notice-cases"],
  ["contact-consent"],
];

export function NoticeDuty({
  item,
  contactId,
}: Readonly<{ item: WorklistItem; contactId: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const caseId = item.id;
  const queryClient = useQueryClient();
  const mayWrite = useCan("contact", "update");
  // The worklist row names whose queue it sits in, not who claimed the duty;
  // only a reader of the privacy queue may ask the case itself.
  const canReadCase = useCan("privacy_request", "read");
  const duty = useQuery({
    queryKey: ["notice-cases", "one", caseId],
    enabled: canReadCase,
    queryFn: async () => {
      const { data, error } = await api.GET("/privacy/notice-cases/{id}", {
        params: { path: { id: caseId } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
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
        <FactList
          className="notice-duty-facts"
          facts={dutyFacts(item, duty.data, t, locale, viewerZone())}
        />
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

// What the deadline rests on, so a precise date with no evidence behind it
// never reads as authoritative.
function dutyFacts(
  item: WorklistItem,
  duty: NoticeCase | undefined,
  t: Translator,
  locale: Locale,
  tz: string,
): Fact[] {
  const date = (iso: string) => formatDate(iso, locale, tz);
  const acquisition = item.acquisition;
  const facts: Fact[] = acquisition
    ? [
        {
          key: "kind",
          term: t("noticeDuty.obtainedAs"),
          value: acquisitionKindLabel(acquisition.kind, t),
        },
        {
          key: "when",
          term: t("noticeDuty.obtainedOn"),
          value: acquisition.occurred_at
            ? date(acquisition.occurred_at)
            : t("noticeDuty.dateUnknown"),
        },
        {
          key: "recorded",
          term: t("noticeDuty.recorded"),
          value: [
            date(acquisition.captured_at),
            acquisitionRecorder(acquisition, t),
          ].join(" · "),
        },
      ]
    : [
        {
          key: "kind",
          term: t("noticeDuty.obtainedAs"),
          value: t("notice.noAcquisition"),
        },
      ];
  if (duty) {
    facts.push({
      key: "owner",
      term: t("notice.owner"),
      value: duty.owner_user_id ? (
        <EntityRef kind="user" id={duty.owner_user_id} />
      ) : (
        t("notice.unassigned")
      ),
    });
  }
  if (item.kind) {
    facts.push({
      key: "rule",
      term: t("noticeDuty.rule"),
      value: noticeRuleLabel(item.kind, t),
      note: noticeRuleHint(item.kind, t),
    });
  }
  if (item.due_at) {
    facts.push({ key: "due", term: t("notice.due"), value: date(item.due_at) });
  }
  return facts;
}
