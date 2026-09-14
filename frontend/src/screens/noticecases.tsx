// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useMemo, useState } from "react";
import { api } from "../api/client";
import { useCan } from "../app/capability";
import {
  Badge,
  Button,
  EmptyState,
  Field,
  SegmentedControl,
  Textarea,
} from "../design-system/atoms";
import { CardBoundary } from "../design-system/cardboundary";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Panel, PanelBody } from "../design-system/panel";
import { Select } from "../design-system/select";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { formatDate } from "../format/format";
import { useNow } from "../format/now";
import { viewerZone } from "../format/timezone";
import { type Locale, useLocale, useT } from "../i18n";
import { humanizeToken } from "./audit";
import {
  problemMessageOf,
  QueryGate,
  QueryStates,
  throwProblem,
  useMe,
} from "./common";
import { useRoster } from "./entityref";
import {
  isNoticeOverdue,
  mayAssign,
  mayExcuse,
  NOTICE_STATES,
  type NoticeCase,
  noticeStateTone,
  UNRESOLVED_NOTICE_STATES,
} from "./noticecases.logic";

// The two facets a privacy officer actually works in. `owed` is the default
// because the queue exists to show duties nobody has discharged; `all` is there
// so a closed case can be found again, which is what an auditor asks for.
//
// BOTH facets name their states explicitly, and "all" especially. The server
// treats an ABSENT state filter as "every unresolved one" — a sensible default
// for a caller that asks for nothing in particular, and exactly wrong here:
// omitting the filter would make "All" return the same rows as "Still owed"
// and quietly hide every case somebody had closed.
const NOTICE_FACETS = ["owed", "all"] as const;
type NoticeFacet = (typeof NOTICE_FACETS)[number];

// What an officer is claiming when they end a duty without sending anything.
// Both require a ground, which is the whole reason they exist beside the older
// `not_required`.
const EXCUSE_STATES = ["provided_elsewhere", "exempt_with_reason"] as const;
type ExcuseState = (typeof EXCUSE_STATES)[number];

// NoticeCasesCard is the disclosure-duty queue: who we obtained without asking,
// whether anybody has told them, and by when we must.
//
// It sits beside the subject-request inbox and behind the same grant, because a
// notice case makes the same kind of disclosure about a named contact that the
// DSR queue makes about somebody exercising a right.
export function NoticeCasesCard() {
  const t = useT();
  const { locale } = useLocale();
  // The only clock touching rendering. isNoticeOverdue stays pure and takes
  // the epoch ms this produces, so a test can drive a deadline without
  // waiting for one.
  const nowMs = useNow(60_000);
  // due_at is a statutory deadline, so a hardcoded zone would show the wrong
  // calendar day to anyone outside it. The viewer's own resolved IANA zone is
  // the only honest answer to "what date does THIS reader see", which is the
  // same call the subject-request queue makes.
  const tz = viewerZone();
  const [facet, setFacet] = useState<NoticeFacet>("owed");
  const [excusing, setExcusing] = useState<NoticeCase | null>(null);
  const queryClient = useQueryClient();

  // `privacy_request:read`, which is what consent/noticeownership.go asks for.
  // Gated rather than merely rendered: the rows name contacts we obtained
  // without asking them, so a reader who came for the consent registry beside
  // this does not issue a call that only 403s.
  const canSee = useCan("privacy_request", "read");
  const canWork = useCan("privacy_request", "update");
  // The probe itself, not only its answer. Capability predicates read off the
  // /me cache and are false while that read is in flight, so branching on
  // `!canSee` alone would flash "not yours" at every officer on every load.
  const me = useMe();

  const query = useQuery({
    queryKey: ["notice-cases", facet],
    enabled: canSee,
    queryFn: async () => {
      const { data, error } = await api.GET("/privacy/notice-cases", {
        params: {
          query: {
            limit: 50,
            // The facet is a SERVER-side filter, not a client re-slice: a
            // re-slice would hide rows the server never told us about and
            // make the count on screen disagree with the one the queue has.
            state:
              facet === "owed"
                ? [...UNRESOLVED_NOTICE_STATES]
                : [...NOTICE_STATES],
          },
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });

  const rows = query.data?.data ?? [];

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["notice-cases"] });
  };

  // The variable carries the id and the owner, rather than the mutationFn
  // reading them out of its closure. useMutation re-arms its options in a
  // passive effect, so a click landing between the commit and that effect runs
  // the PREVIOUS render's closure — which for a row-keyed action means acting
  // on the wrong case, or on none.
  const assign = useMutation({
    mutationFn: async (vars: { id: string; owner: string }) => {
      const { data, error } = await api.POST(
        "/privacy/notice-cases/{id}/assign",
        {
          params: { path: { id: vars.id } },
          body: { owner_user_id: vars.owner },
        },
      );
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: invalidate,
  });

  const excuse = useMutation({
    mutationFn: async (vars: {
      id: string;
      state: ExcuseState;
      note: string;
    }) => {
      const { data, error } = await api.POST(
        "/privacy/notice-cases/{id}/excuse",
        {
          params: { path: { id: vars.id } },
          body: { state: vars.state, resolution_note: vars.note },
        },
      );
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => {
      invalidate();
      setExcusing(null);
    },
  });

  const facetLabels = useMemo(
    () =>
      ({
        owed: t("notice.facetOwed"),
        all: t("notice.facetAll"),
      }) as Record<NoticeFacet, string>,
    [t],
  );

  let body: ReactNode;
  if (!canSee) {
    // Withheld rather than absent. An absent card would read as "no duties",
    // which is a different claim entirely — and the wrong one to make about a
    // compliance queue somebody cannot see.
    body = (
      <QueryGate query={me} pendingLabel={t("notice.readOnlyForPrivacy")}>
        {() => <EmptyState>{t("notice.readOnlyForPrivacy")}</EmptyState>}
      </QueryGate>
    );
  } else {
    body = (
      <QueryStates query={query} pendingLabel={t("notice.facetOwed")}>
        {rows.length === 0 ? (
          <EmptyState>
            {facet === "owed" ? t("notice.emptyOwed") : t("common.empty")}
          </EmptyState>
        ) : (
          <SettingList testId="notice-cases">
            {rows.map((row) => (
              <NoticeRow
                key={row.id}
                row={row}
                nowMs={nowMs}
                locale={locale}
                tz={tz}
                canWork={canWork}
                assignPending={assign.isPending}
                onAssign={(owner) => assign.mutate({ id: row.id, owner })}
                onExcuse={() => setExcusing(row)}
              />
            ))}
          </SettingList>
        )}
      </QueryStates>
    );
  }

  return (
    <Panel
      title={t("notice.title")}
      // The facet rides in the header, above a queue as long as the queue is.
      // As a row it would move every time a duty arrived, which is the same
      // reason the subject-request queue beside this one puts its verb there.
      titleAction={
        canSee ? (
          <SegmentedControl
            options={[...NOTICE_FACETS]}
            value={facet}
            onChange={setFacet}
            labels={facetLabels}
            label={t("notice.facetLabel")}
          />
        ) : null
      }
    >
      <PanelBody>
        <p className="settings-panel-sub">{t("notice.sub")}</p>
        {/* One card's throw stays inside one card: this body renders a queue
            straight off the wire, and without a boundary a single malformed
            row costs the reader the whole tab. */}
        <CardBoundary>
          {body}
          {/* Both write failures surface HERE, not only inside the modal. The
              modal can be dismissed while its submit is still in flight, and
              an error that only rendered there would leave the reader
              believing a duty was excused when it was not. */}
          {assign.isError ? (
            <p className="t-caption">{problemMessageOf(assign.error, t)}</p>
          ) : null}
          {excuse.isError && excusing === null ? (
            <p className="t-caption">{problemMessageOf(excuse.error, t)}</p>
          ) : null}
        </CardBoundary>
        <ExcuseModal
          // Keyed by the case, so opening a second duty MOUNTS a fresh form
          // rather than showing the first one's chosen kind and typed ground.
          // The wrapper stays mounted across open and closed to keep its focus
          // handling, so without this its useState survives the change of
          // subject — and an officer would confirm case B carrying case A's
          // words.
          key={excusing?.id ?? "none"}
          row={excusing}
          onClose={() => setExcusing(null)}
          onConfirm={(state, note) => {
            if (excusing) {
              excuse.mutate({ id: excusing.id, state, note });
            }
          }}
          pending={excuse.isPending}
          error={excuse.isError ? problemMessageOf(excuse.error, t) : null}
        />
      </PanelBody>
    </Panel>
  );
}

// One duty: whose it is, what rule put it there, and by when.
function NoticeRow({
  row,
  nowMs,
  locale,
  tz,
  canWork,
  assignPending,
  onAssign,
  onExcuse,
}: Readonly<{
  row: NoticeCase;
  nowMs: number;
  locale: Locale;
  tz: string;
  canWork: boolean;
  assignPending: boolean;
  onAssign: (owner: string) => void;
  onExcuse: () => void;
}>) {
  const t = useT();
  const overdue = isNoticeOverdue(row.due_at, row.state, nowMs);
  // The roster is fetched only where somebody can actually act on it: a reader
  // who may see the queue but not work it has no use for a list of colleagues.
  const roster = useRoster("user", canWork);
  const options = useMemo(
    () =>
      (roster.data ?? []).map((entry) => ({
        value: entry.id,
        // The roster carries users and teams under one type; only a user can
        // own a duty, and display_name is what a user row carries.
        label: "display_name" in entry ? entry.display_name : entry.id,
      })),
    [roster.data],
  );

  return (
    <SettingRow
      testId={`notice-case-${row.id}`}
      label={
        <>
          <Badge tone={noticeStateTone(row.state)}>
            {humanizeToken(row.state)}
          </Badge>{" "}
          {humanizeToken(row.rule)}
        </>
      }
      description={
        <>
          {t("notice.dueAt", { date: formatDate(row.due_at, locale, tz) })}
          {overdue ? (
            <>
              {" "}
              <Badge tone="danger">{t("notice.overdue")}</Badge>
            </>
          ) : null}
          {row.resolution_note ? <> · {row.resolution_note}</> : null}
        </>
      }
      value={
        // The OWNER answers "who is accountable", never the state. A blocked
        // or queued case can carry one, and an `assigned` case can have lost
        // its owner to a deleted account — which reads as unclaimed, honestly.
        row.owner_user_id ? (
          <Badge>{t("notice.claimed")}</Badge>
        ) : (
          <Badge tone="warn">{t("notice.unclaimed")}</Badge>
        )
      }
      control={
        canWork ? (
          <>
            {mayAssign(row.state) ? (
              // The control shows the CURRENT owner, not a permanent blank.
              // A picker that reset itself after every assign would leave the
              // row saying "Claimed" beside a field naming nobody, and the
              // reader would have to guess whether their click landed.
              <Select
                options={options}
                value={row.owner_user_id ?? ""}
                onChange={onAssign}
                disabled={assignPending || roster.isPending}
                name={`notice-assign-${row.id}`}
              />
            ) : null}
            {mayExcuse(row.state) ? (
              <Button variant="ghost" onClick={onExcuse}>
                {t("notice.excuse")}
              </Button>
            ) : null}
          </>
        ) : undefined
      }
    />
  );
}

// Ending a duty without sending anything, on a ground the officer states.
//
// The ground is required by the server and by this form, which is the whole
// reason these two states exist beside `not_required` — that one records the
// same conclusion with nothing to defend it.
function ExcuseModal({
  row,
  onClose,
  onConfirm,
  pending,
  error,
}: Readonly<{
  row: NoticeCase | null;
  onClose: () => void;
  onConfirm: (state: ExcuseState, note: string) => void;
  pending: boolean;
  error: string | null;
}>) {
  const t = useT();
  const [state, setState] = useState<ExcuseState>("provided_elsewhere");
  const [note, setNote] = useState("");

  const stateOptions = EXCUSE_STATES.map((value) => ({
    value,
    label:
      value === "provided_elsewhere"
        ? t("notice.excuseProvided")
        : t("notice.excuseExempt"),
  }));

  return (
    <ConfirmModal
      open={row !== null}
      onClose={() => {
        setNote("");
        onClose();
      }}
      title={t("notice.excuseTitle")}
      confirmLabel={t("notice.excuseConfirm")}
      // Disabled until there is a ground, because the server refuses without
      // one and a button that fails is worse than one that waits.
      confirmDisabled={note.trim() === ""}
      onConfirm={() => onConfirm(state, note.trim())}
      pending={pending}
      error={error}
    >
      <Field label={t("notice.excuseWhich")}>
        {() => (
          <Select
            options={stateOptions}
            value={state}
            onChange={(value) => setState(value as ExcuseState)}
            name="notice-excuse-state"
          />
        )}
      </Field>
      <Field label={t("notice.excuseGround")}>
        {(control) => (
          <Textarea
            {...control}
            value={note}
            // The server holds 500 characters too. Bounded here as well so a
            // reader learns the limit while typing rather than on submit.
            maxLength={500}
            onChange={(event) => setNote(event.target.value)}
          />
        )}
      </Field>
    </ConfirmModal>
  );
}
