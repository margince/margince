import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { type ReactNode, useId, useMemo, useRef, useState } from "react";
import { api, FIRST_PAGE } from "../../api/client";
import type { components } from "../../api/schema";
import {
  Button,
  EmptyState,
  Field,
  TextInput,
} from "../../design-system/atoms";
import { CardBoundary } from "../../design-system/cardboundary";
import { ConfirmModal } from "../../design-system/confirmmodal";
import {
  ConfirmPhraseBox,
  phraseMatches,
} from "../../design-system/confirmphrase";
import { ErrorLine } from "../../design-system/errorline";
import { FilterPills } from "../../design-system/filterpills";
import { refuseUnreadableField } from "../../design-system/formvalidity";
import { PanelBody, PanelIntro } from "../../design-system/panel";
import {
  LoadMoreButton,
  QueryGate,
  QueryStates,
} from "../../design-system/querystates";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../../design-system/recordpicker";
import { Select } from "../../design-system/select";
import { Stack } from "../../design-system/stack";
import { useNow } from "../../format/now";
import { viewerZone } from "../../format/timezone";
import { useT } from "../../i18n";
import { useCan } from "../../lib/capability";
import { isOption } from "../../lib/options";
import {
  DSR_KIND_LABEL,
  DSR_STATUS_FACETS,
  DSR_STATUS_LABEL,
  type DsrStatusFacet,
  isIllegalTransition,
  isLegalHold,
} from "../../lib/privacy.logic";
import {
  ProblemError,
  problemFieldErrorsOf,
  problemMessageOf,
  unwrap,
} from "../../lib/problem";
import { useMe } from "../../lib/session";
import { useLinkedCase } from "./privacy.caselink";
import { LinkedCaseNotice } from "./privacy.caselink.notice";
import {
  type DeadlineDraft,
  DsrDeadlineFields,
  deadlineBody,
  EMPTY_DEADLINE,
  earlierDueTooLate,
  useRequestDeadline,
} from "./privacy.deadline";
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
type RequestDeadline = components["schemas"]["RequestDeadline"];

const DSR_KINDS: readonly DsrKind[] = ["access", "rectify", "erasure"];

// An erasure resolves subject_ref to a contact id, so it is opened against a
// picked contact; the contact list's own `q` search finds the candidates.
async function searchContactCandidates(
  q: string,
): Promise<RecordPickerCandidate[]> {
  const data = unwrap(
    await api.GET("/contacts", {
      params: { query: { q, limit: 10 } },
    }),
  );
  return data.data.map((contact) => ({
    id: contact.id,
    name: contact.full_name,
  }));
}

type DsrDraft = Readonly<{
  kind: DsrKind;
  subjectRef: string;
  contact: RecordPickerCandidate | null;
  deadline: DeadlineDraft;
}>;

const EMPTY_DSR: DsrDraft = {
  kind: "access",
  subjectRef: "",
  contact: null,
  deadline: EMPTY_DEADLINE,
};

// Kind flips the subject field's shape: an erasure locks onto a picked contact,
// so the form cannot produce the free-text erasure the server refuses.
export function NewDsrForm({ onDone }: Readonly<{ onDone: () => void }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [draft, setDraft] = useState<DsrDraft>(EMPTY_DSR);
  const formId = useId();
  // Minted in the operator's zone, the one the row renders it back in. A bare
  // `new Date(day)` reads the day as UTC midnight and rolls it back west of UTC.
  const tz = viewerZone();

  const deadline = useRequestDeadline(draft.kind, draft.deadline, tz);

  const create = useMutation({
    mutationFn: async ({
      request,
      computed,
    }: {
      request: DsrDraft;
      computed: RequestDeadline;
    }) => {
      const body: CreateDataSubjectRequest = {
        kind: request.kind,
        subject_ref: request.subjectRef.trim(),
        ...deadlineBody(request.deadline, computed, tz),
      };
      return unwrap(
        await api.POST("/data-subject-requests", {
          body,
        }),
      );
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
  const computed = deadline.data;
  const ready =
    draft.subjectRef.trim() !== "" &&
    computed !== undefined &&
    !earlierDueTooLate(draft.deadline, computed, tz);
  return (
    <>
      <form
        id={formId}
        className="form-stack"
        onSubmit={(event) => {
          event.preventDefault();
          if (
            ready &&
            !create.isPending &&
            !refuseUnreadableField(event.currentTarget)
          )
            create.mutate({ request: draft, computed });
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
          <Field
            label={t("privacy.contact")}
            hint={t("privacy.erasureNeedsContact")}
          >
            {(control) => (
              <RecordPicker
                id={control.id}
                aria-describedby={control["aria-describedby"]}
                searchTargets={searchContactCandidates}
                selected={draft.contact}
                onPick={(candidate) =>
                  edit({ contact: candidate, subjectRef: candidate.id })
                }
              />
            )}
          </Field>
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

        <DsrDeadlineFields
          draft={draft.deadline}
          query={deadline}
          refused={dueRefused}
          zone={tz}
          onChange={(next) =>
            edit({ deadline: { ...draft.deadline, ...next } })
          }
        />

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

// The word that fulfils an erasure. It stays English in every locale.
const ERASE_PHRASE = "ERASE";

// Fulfilling an erasure wipes a contact across the whole system, so it waits on
// a typed ERASE_PHRASE. A legal-hold 409 is a lawful refusal, told apart from a
// fault.
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
  const phraseInput = useRef<HTMLInputElement>(null);

  const patch = useMutation({
    mutationFn: async (
      fulfilment: Readonly<{ request: DataSubjectRequest; resolution: string }>,
    ) => {
      const body: UpdateDataSubjectRequest = { status: "fulfilled" };
      // Omitted when blank: an empty string is a value the server would write.
      if (fulfilment.resolution.trim()) {
        body.resolution = fulfilment.resolution.trim();
      }
      return unwrap(
        await api.PATCH("/data-subject-requests/{id}", {
          params: { path: { id: fulfilment.request.id } },
          body,
        }),
      );
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
  // Neither a hold nor a race is a mistake a retry fixes. Both keep their own
  // sentence and leave the generic slot to everything else.
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
      // A reported hold or a request that moved on fails the same way on every
      // retry. So the confirm stays shut until the operator reopens the dialog
      // against the request's current state.
      confirmDisabled={!phraseMatches(typed, ERASE_PHRASE) || held || movedOn}
      onConfirm={() => dsr && patch.mutate({ request: dsr, resolution })}
      pending={patch.isPending}
      error={errorMessage}
      returnFocusTo={returnFocusTo}
      initialFocusTo={() => phraseInput.current}
    >
      <p>{t("privacy.erasureIrreversible")}</p>
      <ConfirmPhraseBox
        prompt={t("privacy.typeErasePrompt")}
        phrase={ERASE_PHRASE}
        value={typed}
        onChange={setTyped}
        inputRef={phraseInput}
      />
      <ErasureRefusals held={held} movedOn={movedOn} />
    </ConfirmModal>
  );
}

// The subject-request lane of the Requests panel: its bands, not a panel.
export function PrivacyInboxCard() {
  const t = useT();
  // The only clock touching rendering; isOverdue stays pure.
  const nowMs = useNow(60_000);
  const [facet, setFacet] = useState<DsrStatusFacet>("all");
  const detailTitleId = useId();
  // One erasure confirm at the card root, carrying the answer the drawer drafted.
  const [fulfilling, setFulfilling] = useState<{
    dsr: DataSubjectRequest;
    resolution: string;
  } | null>(null);

  // `privacy_request:read`: the rows name whoever exercised an Art. 15/17
  // right, so the read is gated and never issued without the grant.
  const canSee = useCan("privacy_request", "read");
  // The probe itself: every grant reads false while /me is in flight.
  const me = useMe();

  // Server-side facet: a client re-slice would break the pager's has_more.
  const query = useInfiniteQuery({
    queryKey: ["dsrs", facet],
    enabled: canSee,
    initialPageParam: FIRST_PAGE,
    queryFn: async ({ pageParam }) => {
      return unwrap(
        await api.GET("/data-subject-requests", {
          params: {
            query: {
              limit: 20,
              ...(facet !== "all" ? { status: facet } : {}),
              ...(pageParam ? { cursor: pageParam } : {}),
            },
          },
        }),
      );
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
  const facetPills = useMemo(
    () =>
      DSR_STATUS_FACETS.map((value) => ({
        value,
        label:
          value === "all" ? t("privacy.facetAll") : t(DSR_STATUS_LABEL[value]),
      })),
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
        <EmptyState>
          {facet === "all" ? t("privacy.requestsEmpty") : t("common.empty")}
        </EmptyState>
      </QueryStates>
    );
  }

  return (
    <>
      <PanelBody>
        <PanelIntro>{t("settings.privacySub")}</PanelIntro>
        <Stack gap="3">
          {/* Once a cut is chosen it stays, so an empty cut can be left again. */}
          {canSee && (facet !== "all" || rows.length > 0) && (
            <FilterPills
              pills={facetPills}
              value={facet}
              onChange={setFacet}
              label={t("privacy.facetLabel")}
            />
          )}
          <LinkedCaseNotice linked={linked} />
          {body}
        </Stack>
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
      <FulfilErasureModal
        dsr={fulfilling?.dsr ?? null}
        resolution={fulfilling?.resolution ?? ""}
        onClose={() => setFulfilling(null)}
        // The fulfil verb is gone once the request closes; the drawer's title stays.
        returnFocusTo={() => document.getElementById(detailTitleId)}
      />
    </>
  );
}
