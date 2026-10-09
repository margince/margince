import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { type ReactNode, useId, useMemo, useState } from "react";
import { api, FIRST_PAGE } from "../api/client";
import type { components } from "../api/schema";
import { useCan, useCanWrite } from "../app/capability";
import { isOption } from "../app/options";
import {
  Button,
  EmptyState,
  Field,
  Modal,
  SegmentedControl,
  TextInput,
} from "../design-system/atoms";
import { CardBoundary } from "../design-system/cardboundary";
import { ConfirmModal } from "../design-system/confirmmodal";
import { ErrorLine } from "../design-system/errorline";
import { Heading } from "../design-system/heading";
import { Panel, PanelBody, PanelIntro } from "../design-system/panel";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../design-system/recordpicker";
import { Select } from "../design-system/select";
import { useNow } from "../format/now";
import { viewerZone } from "../format/timezone";
import { useT } from "../i18n";
import {
  LoadMoreButton,
  ProblemError,
  problemFieldErrorsOf,
  problemMessageOf,
  QueryGate,
  QueryStates,
  throwProblem,
  useMe,
} from "./common";
import { useLinkedCase } from "./privacy.caselink";
import { LinkedCaseNotice } from "./privacy.caselink.notice";
import {
  DSR_KIND_LABEL,
  DSR_STATUS_FACETS,
  DSR_STATUS_LABEL,
  type DsrStatusFacet,
  endOfDayInZone,
  isIllegalTransition,
  isLegalHold,
} from "./privacy.logic";
import { ErasureRefusals } from "./privacy.notices";
import {
  type DataSubjectRequest,
  DsrDetail,
  DsrTable,
} from "./privacy.requests";
import "./privacy.css";

type CreateDataSubjectRequest =
  components["schemas"]["CreateDataSubjectRequest"];
type UpdateDataSubjectRequest =
  components["schemas"]["UpdateDataSubjectRequest"];
type DsrKind = CreateDataSubjectRequest["kind"];

const DSR_KINDS: readonly DsrKind[] = ["access", "rectify", "erasure"];

// An erasure resolves subject_ref to a contact id, so it is opened against a
// picked contact; the contact list's own `q` search finds the candidates.
async function searchContactCandidates(
  q: string,
): Promise<RecordPickerCandidate[]> {
  const { data, error } = await api.GET("/contacts", {
    params: { query: { q, limit: 10 } },
  });
  if (error) {
    throwProblem(error);
  }
  return data.data.map((contact) => ({
    id: contact.id,
    name: contact.full_name,
  }));
}

type DsrDraft = Readonly<{
  kind: DsrKind;
  subjectRef: string;
  contact: RecordPickerCandidate | null;
  dueAt: string;
}>;

const EMPTY_DSR: DsrDraft = {
  kind: "access",
  subjectRef: "",
  contact: null,
  dueAt: "",
};

// Kind flips the subject field's shape: an erasure locks onto a picked contact,
// so the form cannot produce the free-text erasure the server refuses.
function NewDsrForm({ onDone }: Readonly<{ onDone: () => void }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<DsrDraft>(EMPTY_DSR);
  const formId = useId();
  // Minted in the operator's zone, the one the row renders it back in; a bare
  // `new Date(day)` reads the day as UTC midnight and rolls it back west of UTC.
  const tz = viewerZone();

  const create = useMutation({
    mutationFn: async (request: DsrDraft) => {
      const body: CreateDataSubjectRequest = {
        kind: request.kind,
        subject_ref: request.subjectRef.trim(),
        due_at: endOfDayInZone(request.dueAt, tz),
      };
      const { data, error } = await api.POST("/data-subject-requests", {
        body,
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ["dsrs"] });
      setDraft(EMPTY_DSR);
      onDone();
    },
  });

  function edit(next: Partial<DsrDraft>) {
    setDraft((was) => ({ ...was, ...next }));
    if (create.isError) {
      create.reset();
    }
  }

  const dueRefused = problemFieldErrorsOf(create.error).some(
    (problem) => problem.field === "due_at",
  );
  const ready = draft.subjectRef.trim() !== "" && draft.dueAt !== "";
  return (
    <>
      <form
        id={formId}
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          if (ready && !create.isPending) create.mutate(draft);
        }}
      >
        <Field label={t("privacy.kind")}>
          {(control) => (
            <Select
              {...control}
              options={DSR_KINDS.map((value) => ({
                value,
                label: t(DSR_KIND_LABEL[value]),
              }))}
              value={draft.kind}
              onChange={(value) => {
                // Neither subject shape carries across: a stale value of the
                // other shape would ride into the request unnoticed.
                if (isOption(value, DSR_KINDS)) {
                  edit({ kind: value, subjectRef: "", contact: null });
                }
              }}
            />
          )}
        </Field>

        {draft.kind === "erasure" ? (
          <div className="field">
            <span className="t-label">{t("privacy.contact")}</span>
            <RecordPicker
              label={t("privacy.contact")}
              searchTargets={searchContactCandidates}
              selected={draft.contact}
              onPick={(candidate) =>
                edit({ contact: candidate, subjectRef: candidate.id })
              }
            />
            <p className="t-caption">{t("privacy.erasureNeedsContact")}</p>
          </div>
        ) : (
          <Field
            label={t("privacy.subjectRef")}
            hint={
              draft.kind === "access" ? t("privacy.accessManual") : undefined
            }
          >
            {(control) => (
              <TextInput
                {...control}
                value={draft.subjectRef}
                onChange={(event) => edit({ subjectRef: event.target.value })}
              />
            )}
          </Field>
        )}

        <Field
          label={t("privacy.dueAt")}
          required
          error={dueRefused ? t("privacy.dueRequired") : undefined}
        >
          {(control) => (
            <TextInput
              {...control}
              type="date"
              value={draft.dueAt}
              onChange={(event) => edit({ dueAt: event.target.value })}
            />
          )}
        </Field>

        {!dueRefused && <ErrorLine error={create.error} />}
      </form>
      <div className="actions">
        <Button
          type="submit"
          form={formId}
          variant="primary"
          disabled={!ready || create.isPending}
        >
          {t("privacy.openRequest")}
        </Button>
      </div>
    </>
  );
}

// Fulfilling an erasure wipes a contact across the whole system, so it waits on
// a typed ERASE. A legal-hold 409 is a lawful refusal, told apart from a fault.
function FulfilErasureModal({
  dsr,
  resolution,
  onClose,
  returnFocusTo,
}: Readonly<{
  dsr: DataSubjectRequest | null;
  resolution: string;
  onClose: () => void;
  returnFocusTo: () => HTMLElement | null;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [typed, setTyped] = useState("");

  const patch = useMutation({
    mutationFn: async (
      fulfilment: Readonly<{ request: DataSubjectRequest; resolution: string }>,
    ) => {
      const body: UpdateDataSubjectRequest = { status: "fulfilled" };
      // Omitted when blank: an empty string is a value the server would write.
      if (fulfilment.resolution.trim()) {
        body.resolution = fulfilment.resolution.trim();
      }
      const { data, error } = await api.PATCH("/data-subject-requests/{id}", {
        params: { path: { id: fulfilment.request.id } },
        body,
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: async () => {
      // The queue first, so focus lands on a drawer that already reads fulfilled.
      await queryClient.invalidateQueries({ queryKey: ["dsrs"] });
      setTyped("");
      onClose();
    },
    onError: (error) => {
      if (error instanceof ProblemError && isIllegalTransition(error.problem)) {
        void queryClient.invalidateQueries({ queryKey: ["dsrs"] });
      }
    },
  });

  function close() {
    onClose();
    setTyped("");
    patch.reset();
  }

  const problem =
    patch.error instanceof ProblemError ? patch.error.problem : null;
  const held = problem !== null && isLegalHold(problem);
  const movedOn = problem !== null && isIllegalTransition(problem);
  // Neither a hold nor a race is a mistake a retry fixes, so both keep their
  // own sentence and leave the generic slot to everything else.
  const errorMessage =
    patch.isError && !held && !movedOn
      ? problemMessageOf(patch.error, t)
      : null;

  return (
    <ConfirmModal
      open={dsr !== null}
      onClose={close}
      title={t("privacy.fulfilErasureTitle")}
      confirmLabel={t("privacy.erasureConfirm")}
      confirmVariant="danger"
      confirmDisabled={
        typed.trim().toUpperCase() !== "ERASE" || held || movedOn
      }
      onConfirm={() => dsr && patch.mutate({ request: dsr, resolution })}
      pending={patch.isPending}
      error={errorMessage}
      returnFocusTo={returnFocusTo}
    >
      <p>{t("privacy.erasureIrreversible")}</p>
      <Field label={t("privacy.typeErase")}>
        {(control) => (
          <TextInput
            {...control}
            value={typed}
            onChange={(event) => setTyped(event.target.value)}
          />
        )}
      </Field>
      <ErasureRefusals held={held} movedOn={movedOn} />
    </ConfirmModal>
  );
}

export function PrivacyInboxCard() {
  const t = useT();
  // The only clock touching rendering; isOverdue stays pure.
  const nowMs = useNow(60_000);
  const [facet, setFacet] = useState<DsrStatusFacet>("all");
  const createTitleId = useId();
  const detailTitleId = useId();
  const [creating, setCreating] = useState(false);
  // One erasure confirm at the card root, carrying the answer the drawer drafted.
  const [fulfilling, setFulfilling] = useState<{
    dsr: DataSubjectRequest;
    resolution: string;
  } | null>(null);

  // `privacy_request:read`: the rows name whoever exercised an Art. 15/17
  // right, so the read is gated and never issued without the grant.
  const canSee = useCan("privacy_request", "read");
  // Opening a request writes the contact it names (consent/dsr.go CreateDSR).
  const canOpenRequest = useCanWrite("contact", "update");
  // The probe itself: every grant reads false while /me is in flight.
  const me = useMe();

  // Server-side facet: a client re-slice would break the pager's has_more.
  const query = useInfiniteQuery({
    queryKey: ["dsrs", facet],
    enabled: canSee,
    initialPageParam: FIRST_PAGE,
    queryFn: async ({ pageParam }) => {
      const { data, error } = await api.GET("/data-subject-requests", {
        params: {
          query: {
            limit: 20,
            ...(facet !== "all" ? { status: facet } : {}),
            ...(pageParam ? { cursor: pageParam } : {}),
          },
        },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    getNextPageParam: (last) => last.page.next_cursor ?? null,
  });

  // Memoised against the minute tick, which would otherwise re-flatten every
  // page and hand the facet bar a new labels object each minute.
  const pages = query.data?.pages;
  const rows = useMemo(
    () => pages?.flatMap((page) => page.data) ?? [],
    [pages],
  );
  // The open request is the address: the worklist links here naming one.
  const { expandedId, linked, toggle } = useLinkedCase(
    useMemo(() => rows.map((dsr) => dsr.id), [rows]),
    query.hasNextPage && !query.isFetchingNextPage,
    query.fetchNextPage,
  );
  const open = rows.find((dsr) => dsr.id === expandedId) ?? null;
  const facetLabels = useMemo(
    (): Record<DsrStatusFacet, string> => ({
      all: t("privacy.facetAll"),
      open: t(DSR_STATUS_LABEL.open),
      in_progress: t(DSR_STATUS_LABEL.in_progress),
      fulfilled: t(DSR_STATUS_LABEL.fulfilled),
      rejected: t(DSR_STATUS_LABEL.rejected),
    }),
    [t],
  );

  let body: ReactNode = null;
  if (!canSee) {
    // Withheld rather than absent: an absent card would read as "no requests".
    body = (
      <QueryGate query={me} pendingLabel={t("privacy.inboxAdminOnly")}>
        {() => <EmptyState>{t("privacy.inboxAdminOnly")}</EmptyState>}
      </QueryGate>
    );
  } else if (!query.isSuccess || rows.length === 0) {
    body = (
      <QueryStates query={query} pendingLabel={t("privacy.loading")}>
        <EmptyState>{t("common.empty")}</EmptyState>
      </QueryStates>
    );
  }

  return (
    <Panel
      title={t("settings.privacy")}
      // Opening a request asks `contact:update`, not the queue's grant.
      titleAction={
        canOpenRequest ? (
          <Button aria-haspopup="dialog" onClick={() => setCreating(true)}>
            {t("privacy.newRequest")}
          </Button>
        ) : null
      }
    >
      <PanelBody>
        <PanelIntro>{t("settings.privacySub")}</PanelIntro>
        {canSee && (
          <div className="filter-tabs">
            <SegmentedControl
              options={DSR_STATUS_FACETS}
              value={facet}
              onChange={setFacet}
              labels={facetLabels}
              label={t("privacy.facetLabel")}
            />
          </div>
        )}
        <LinkedCaseNotice linked={linked} />
        {body}
      </PanelBody>
      <CardBoundary>
        {body === null && (
          <>
            <DsrTable rows={rows} nowMs={nowMs} onOpen={toggle} />
            {query.hasNextPage && (
              <PanelBody>
                <LoadMoreButton query={query} />
              </PanelBody>
            )}
          </>
        )}
        {open && (
          <DsrDetail
            key={open.id}
            dsr={open}
            titleId={detailTitleId}
            nowMs={nowMs}
            onClose={() => toggle(open.id)}
            onFulfilErasure={(dsr, resolution) =>
              setFulfilling({ dsr, resolution })
            }
          />
        )}
      </CardBoundary>
      <Modal
        open={creating}
        onClose={() => setCreating(false)}
        labelledBy={createTitleId}
        intent="form"
      >
        <Heading size="large" id={createTitleId} className="t-h2 modal-title">
          {t("privacy.newRequest")}
        </Heading>
        <NewDsrForm onDone={() => setCreating(false)} />
      </Modal>
      <FulfilErasureModal
        dsr={fulfilling?.dsr ?? null}
        resolution={fulfilling?.resolution ?? ""}
        onClose={() => setFulfilling(null)}
        // The fulfil verb is gone once the request closes; the drawer's title stays.
        returnFocusTo={() => document.getElementById(detailTitleId)}
      />
    </Panel>
  );
}
