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
import { api } from "../../api/client";
import { EntityRef } from "../../components/entityref";
import { Button, EmptyState } from "../../design-system/atoms";
import { type Fact, FactList } from "../../design-system/factlist";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryStates } from "../../design-system/querystates";
import { formatDate } from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { type Locale, type Translator, useLocale, useT } from "../../i18n";
import { useCan, useCanWrite } from "../../lib/capability";
import { problemMessageOf, unwrap } from "../../lib/problem";
import type { WorklistItem } from "../worklist/worklist.queries";
import {
  ControllerMailDialog,
  type ControllerMailIssued,
  type ControllerMailKind,
} from "./controllermail";
import {
  acquisitionKindLabel,
  acquisitionRecorder,
  noticeRuleHint,
  noticeRuleLabel,
} from "./noticeacquisition";
import {
  isNoticeResolved,
  NOTICE_STATE_LABEL,
  type NoticeCase,
} from "./noticecases.logic";
import { ExcuseModal, type ExcuseState } from "./noticeexcuse";
import { NoticeSourceReview } from "./noticesource";

// Every read that could still show the duty: the Home brief, the worklist the
// Focus list draws from, the settings queue and the contact's consent panel.
const DUTY_READS = [
  ["brief"],
  ["worklist"],
  ["notice-cases"],
  ["contact-consent"],
  ["contact360"],
];

export function NoticeDuty(
  props: Readonly<
    { item: WorklistItem; contactId: string } | { caseId: string }
  >,
) {
  const t = useT();
  const { locale } = useLocale();
  const item = "item" in props ? props.item : undefined;
  const caseId = "caseId" in props ? props.caseId : props.item.id;
  const queryClient = useQueryClient();
  const mayWrite = useCanWrite("contact", "update");
  const mayExcuse = useCanWrite("privacy_request", "update");
  // The worklist row names whose queue it sits in, not who claimed the duty.
  // Only a reader of the privacy queue may ask the case itself.
  const canReadCase = useCan("privacy_request", "read");
  const duty = useQuery({
    queryKey: ["notice-cases", "one", caseId],
    enabled: canReadCase,
    queryFn: async () => {
      return unwrap(
        await api.GET("/privacy/notice-case-reviews/{id}", {
          params: { path: { id: caseId } },
        }),
      );
    },
  });
  const contactId =
    "contactId" in props ? props.contactId : duty.data?.contact_id;
  const owed = duty.data
    ? !isNoticeResolved(duty.data.state) &&
      duty.data.state !== "queued" &&
      duty.data.state !== "source_review"
    : item !== undefined;
  const canAct = contactId && duty.data && owed;
  const [excusing, setExcusing] = useState(false);
  const [done, setDone] = useState<string | null>(null);
  const settle = (sentence: string) => {
    setDone(sentence);
    for (const key of DUTY_READS) {
      void queryClient.invalidateQueries({ queryKey: key });
    }
  };

  // The kind outlives the close, so the dialog keeps its words while it leaves.
  const [mailKind, setMailKind] =
    useState<ControllerMailKind>("privacy_notice");
  const [previewing, setPreviewing] = useState(false);
  const preview = (kind: ControllerMailKind) => {
    setMailKind(kind);
    setPreviewing(true);
  };
  const reportSent = (issued: ControllerMailIssued) =>
    settle(
      issued.queued
        ? t("noticeDuty.sent", { address: issued.delivered_to })
        : t("noticeDuty.notSent", { address: issued.delivered_to }),
    );

  const excuse = useMutation({
    mutationFn: async (vars: { state: ExcuseState; note: string }) => {
      unwrap(
        await api.POST("/privacy/notice-cases/{id}/excuse", {
          params: { path: { id: caseId } },
          body: { state: vars.state, resolution_note: vars.note },
        }),
      );
    },
    onSuccess: () => {
      setExcusing(false);
      settle(t("noticeDuty.ended"));
    },
  });

  if (!item && !duty.data) {
    return (
      <Panel title={t("notice.title")}>
        <PanelBody>
          {canReadCase ? (
            <QueryStates query={duty} pendingLabel={t("notice.loading")}>
              {null}
            </QueryStates>
          ) : (
            <EmptyState>{t("notice.readOnlyForPrivacy")}</EmptyState>
          )}
        </PanelBody>
      </Panel>
    );
  }
  if (duty.data?.state === "source_review")
    return <NoticeSourceReview key={duty.data.id} duty={duty.data} />;
  return (
    <Panel title={noticeDutyTitle(duty.data, owed, t)}>
      <PanelBody>
        {owed && <p>{t("noticeDuty.what")}</p>}
        <FactList
          className="notice-duty-facts"
          facts={dutyFacts(item, duty.data, t, locale, viewerZone())}
        />
        {owed && <p className="t-caption">{t("noticeDuty.how")}</p>}
        <NoticeDutyActions
          done={done}
          canAct={Boolean(canAct)}
          mayWrite={mayWrite}
          mayExcuse={mayExcuse}
          t={t}
          preview={preview}
          onExcuse={() => setExcusing(true)}
        />
        {contactId && (
          <ControllerMailDialog
            contactId={contactId}
            kind={mailKind}
            open={previewing}
            onClose={() => setPreviewing(false)}
            onSent={reportSent}
          />
        )}
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

function noticeDutyTitle(
  duty: NoticeCase | undefined,
  owed: boolean,
  t: Translator,
) {
  return t(
    duty?.rule === "art13" || (duty && !owed)
      ? "notice.title"
      : "noticeDuty.title",
  );
}

function NoticeDutyActions({
  done,
  canAct,
  mayWrite,
  mayExcuse,
  t,
  preview,
  onExcuse,
}: Readonly<{
  done: string | null;
  canAct: boolean;
  mayWrite: boolean;
  mayExcuse: boolean;
  t: Translator;
  preview: (kind: ControllerMailKind) => void;
  onExcuse: () => void;
}>) {
  if (done) return <p role="status">{done}</p>;
  if (!canAct) return null;
  return (
    <div className="notice-duty-actions">
      {mayWrite && (
        <>
          <Button
            variant="primary"
            aria-haspopup="dialog"
            onClick={() => preview("privacy_notice")}
          >
            {t("noticeDuty.sendNotice")}
          </Button>
          <Button
            aria-haspopup="dialog"
            onClick={() => preview("record_confirmation")}
          >
            {t("noticeDuty.askConfirm")}
          </Button>
        </>
      )}
      {mayExcuse && <Button onClick={onExcuse}>{t("noticeDuty.end")}</Button>}
    </div>
  );
}

// What the deadline rests on, so a precise date with no evidence behind it
// never reads as authoritative.
function dutyFacts(
  item: WorklistItem | undefined,
  duty: NoticeCase | undefined,
  t: Translator,
  locale: Locale,
  tz: string,
): Fact[] {
  const date = (iso: string) => formatDate(iso, locale, tz);
  const acquisition = duty?.acquisition ?? item?.acquisition;
  const facts = acquisitionFacts(acquisition, t, date);
  if (!item && duty?.contact_name) {
    facts.unshift({
      key: "contact",
      term: t("notice.contact"),
      value: (
        <EntityRef
          kind="contact"
          id={duty.contact_id}
          name={duty.contact_name}
        />
      ),
    });
  }
  if (duty) {
    facts.push({
      key: "state",
      term: t("history.field.status"),
      value: t(NOTICE_STATE_LABEL[duty.state]),
    });
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
  facts.push(...sourceAssessmentFacts(duty, t, date));
  const rule = duty?.rule ?? item?.kind;
  if (rule) {
    facts.push({
      key: "rule",
      term: t("noticeDuty.rule"),
      value: noticeRuleLabel(rule, t),
      note: noticeRuleHint(rule, t),
    });
  }
  return [...facts, ...deadlineFacts(item, duty, acquisition, t, date)];
}

function deadlineFacts(
  item: WorklistItem | undefined,
  duty: NoticeCase | undefined,
  acquisition: NoticeCase["acquisition"],
  t: Translator,
  date: (iso: string) => string,
): Fact[] {
  const facts: Fact[] = [];
  const due = duty
    ? duty.due_at
    : item && item.source === "notice_case"
      ? item.due_at
      : undefined;
  if (due) {
    facts.push({
      key: "due",
      term: t(
        acquisition?.deadline_is_upper_bound
          ? "notice.dueLatest"
          : "notice.due",
      ),
      value: date(due),
      note: acquisition?.deadline_is_upper_bound
        ? t("notice.unknownDateWarning")
        : undefined,
    });
  }
  return facts;
}

function sourceAssessmentFacts(
  duty: NoticeCase | undefined,
  t: Translator,
  date: (iso: string) => string,
): Fact[] {
  if (!duty?.source_kind) return [];
  const facts: Fact[] = [
    {
      key: "confirmed-source",
      term: t("notice.source.confirm"),
      value: acquisitionKindLabel(duty.source_kind, t),
    },
  ];
  if (duty.source_obtained_at)
    facts.push({
      key: "confirmed-date",
      term: t("noticeDuty.obtainedOn"),
      value: date(duty.source_obtained_at),
    });
  if (duty.source_review_note)
    facts.push({
      key: "source-evidence",
      term: t("notice.source.evidence"),
      value: duty.source_review_note,
    });
  return facts;
}

function acquisitionFacts(
  acquisition: NoticeCase["acquisition"],
  t: Translator,
  date: (iso: string) => string,
): Fact[] {
  return acquisition
    ? [
        {
          key: "kind",
          term: t("noticeDuty.obtainedAs"),
          value: acquisitionKindLabel(acquisition.kind, t),
        },
        {
          key: "when",
          term: t(
            acquisition.kind === "unknown_legacy"
              ? "noticeDuty.observedOn"
              : "noticeDuty.obtainedOn",
          ),
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
}
