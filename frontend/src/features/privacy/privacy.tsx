import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { type ReactNode, useId, useMemo, useRef, useState } from "react";
import { api, FIRST_PAGE } from "../../api/client";
import { EmptyState } from "../../design-system/atoms";
import { CardBoundary } from "../../design-system/cardboundary";
import { ConfirmModal } from "../../design-system/confirmmodal";
import {
  ConfirmPhraseBox,
  phraseMatches,
} from "../../design-system/confirmphrase";
import { FilterPills } from "../../design-system/filterpills";
import { PanelBody, PanelIntro } from "../../design-system/panel";
import {
  LoadMoreButton,
  QueryGate,
  QueryStates,
} from "../../design-system/querystates";
import { Stack } from "../../design-system/stack";
import { useNow } from "../../format/now";
import { useT } from "../../i18n";
import { useCan } from "../../lib/capability";
import {
  closingBody,
  DSR_STATUS_FACETS,
  DSR_STATUS_LABEL,
  type DsrClosing,
  type DsrStatusFacet,
  isIllegalTransition,
  isLegalHold,
} from "../../lib/privacy.logic";
import { useLinkedCase } from "../../lib/privacycaselink";
import { ProblemError, problemMessageOf, unwrap } from "../../lib/problem";
import { useMe } from "../../lib/session";
import { dsrsKey } from "../../lib/workkeys";
import { LinkedCaseNotice } from "./privacy.caselink.notice";
import { ErasureRefusals } from "./privacy.notices";
import {
  type DataSubjectRequest,
  DsrDetail,
  DsrTable,
} from "./privacy.requests";
import "./privacy.css";

// The settings screen reaches the privacy feature through this file alone.
export { NewDsrForm } from "./privacy.newrequest";

// The word that fulfils an erasure. It stays English in every locale.
const ERASE_PHRASE = "ERASE";

// Fulfilling an erasure wipes a contact across the whole system, so it waits on
// a typed ERASE_PHRASE. A legal-hold 409 is a lawful refusal, told apart from a
// fault.
function FulfilErasureModal({
  dsr,
  closing,
  onClose,
  returnFocusTo,
}: Readonly<{
  dsr: DataSubjectRequest | null;
  closing: DsrClosing | null;
  onClose: () => void;
  returnFocusTo: () => HTMLElement | null;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [typed, setTyped] = useState("");
  const phraseInput = useRef<HTMLInputElement>(null);

  const patch = useMutation({
    mutationFn: async (
      fulfilment: Readonly<{
        request: DataSubjectRequest;
        closing: DsrClosing;
      }>,
    ) => {
      return unwrap(
        await api.PATCH("/data-subject-requests/{id}", {
          params: { path: { id: fulfilment.request.id } },
          body: closingBody("fulfilled", fulfilment.closing, "erasure"),
        }),
      );
    },
    onSuccess: async () => {
      // The queue first, so focus lands on a drawer that already reads fulfilled.
      await queryClient.invalidateQueries({ queryKey: dsrsKey });
      setTyped("");
      onClose();
    },
    onError: (error) => {
      if (error instanceof ProblemError && isIllegalTransition(error.problem)) {
        void queryClient.invalidateQueries({ queryKey: dsrsKey });
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
      onConfirm={() =>
        dsr && closing && patch.mutate({ request: dsr, closing })
      }
      pending={patch.isPending}
      error={errorMessage}
      returnFocusTo={returnFocusTo}
      initialFocusTo={() => phraseInput.current}
    >
      <p>{t("privacy.erasureIrreversible")}</p>
      {/* A typed draft is not kept: said here, before the press, so nothing
          the officer wrote goes without a word. */}
      {closing?.resolution.trim() && <p>{t("privacy.erasureDraftReplaced")}</p>}
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
    closing: DsrClosing;
  } | null>(null);

  // `privacy_request:read`: the rows name whoever exercised an Art. 15/17
  // right, so the read is gated and never issued without the grant.
  const canSee = useCan("privacy_request", "read");
  // The probe itself: every grant reads false while /me is in flight.
  const me = useMe();

  // Server-side facet: a client re-slice would break the pager's has_more.
  const query = useInfiniteQuery({
    queryKey: [...dsrsKey, facet],
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
            onFulfilErasure={(dsr, closing) => setFulfilling({ dsr, closing })}
          />
        )}
      </CardBoundary>
      <FulfilErasureModal
        dsr={fulfilling?.dsr ?? null}
        closing={fulfilling?.closing ?? null}
        onClose={() => setFulfilling(null)}
        // The fulfil verb is gone once the request closes; the drawer's title stays.
        returnFocusTo={() => document.getElementById(detailTitleId)}
      />
    </>
  );
}
