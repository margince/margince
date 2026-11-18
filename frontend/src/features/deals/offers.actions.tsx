import {
  type QueryClient,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { Check, RefreshCw, Send, Trash2, X } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/client";
import { useIdempotencyKey } from "../../api/idempotencykey";
import { ifMatch, requireVersion } from "../../api/version";
import { Button, Field, TextInput } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody } from "../../design-system/panel";
import { useT } from "../../i18n";
import { fromHere } from "../../lib/navigatefrom";
import { problemMessageOf, throwProblem, unwrap } from "../../lib/problem";
import { reloadReportingFigures } from "../../lib/reportingfigures";
import { dealProductsKey } from "./deal360/dealproducts";
import type { Offer } from "./offers";

// The send/accept/reject lifecycle. All three return the full updated Offer
// (server-truth totals and status), so success only calls
// queryClient.setQueryData(["offer", ...]), never a status flipped locally.
// Send is the confirm-first (🟡) action: a human's own click on this REST path
// is the approval. So no ApprovalToken or Idempotency-Key is sent here; that
// plumbing belongs to the agent/passport path.
// A refused lifecycle step most often means the offer moved on elsewhere (a
// newer revision, another tab, an agent), so the page re-reads its status.
function rereadOffer(queryClient: QueryClient, id: string) {
  return queryClient.invalidateQueries({ queryKey: ["offer", id] });
}

export function SendOfferAction({ offer }: Readonly<{ offer: Offer }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);

  const mutation = useMutation({
    mutationFn: async () => {
      return unwrap(
        await api.POST("/offers/{id}/send", {
          params: {
            path: { id: offer.id },
            ...ifMatch(requireVersion(offer.version)),
          },
        }),
      );
    },
    onSuccess: (data) => {
      queryClient.setQueryData(["offer", offer.id], data);
      setOpen(false);
    },
    onError: () => rereadOffer(queryClient, offer.id),
  });

  const errorMessage = mutation.isError
    ? problemMessageOf(mutation.error, t)
    : null;

  return (
    <>
      <Button
        variant="primary"
        data-testid="send-offer"
        onClick={() => setOpen(true)}
      >
        <Send aria-hidden /> {t("offer.send")}
      </Button>
      <ConfirmModal
        open={open}
        onClose={() => {
          mutation.reset();
          setOpen(false);
        }}
        title={t("offer.sendConfirm")}
        tier="confirm"
        // The confirm names its write: send, accept and reject sit one press
        // apart, and three buttons all reading "Confirm" cannot be told apart.
        confirmLabel={t("offer.send")}
        onConfirm={() => mutation.mutate()}
        pending={mutation.isPending}
        error={errorMessage}
      >
        <p className="t-body">{t("offer.sendBody")}</p>
      </ConfirmModal>
    </>
  );
}

// Accept is human-only. The server syncs the deal's amount and currency on
// acceptance, and derives the deal's products from the offer. So the deal
// screen's ["deal", id], ["deal-offers", id] and ["deal-products", id] refetch,
// and so do the report figures that read the amount and the products.
// setQueryData alone would leave stale figures when the reader goes back.
export function AcceptOfferAction({ offer }: Readonly<{ offer: Offer }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);

  const mutation = useMutation({
    mutationFn: async () => {
      return unwrap(
        await api.POST("/offers/{id}/accept", {
          params: {
            path: { id: offer.id },
            ...ifMatch(requireVersion(offer.version)),
          },
        }),
      );
    },
    onSuccess: (data) => {
      queryClient.setQueryData(["offer", offer.id], data);
      queryClient.invalidateQueries({ queryKey: ["deal", offer.deal_id] });
      queryClient.invalidateQueries({
        queryKey: ["deal-offers", offer.deal_id],
      });
      queryClient.invalidateQueries({
        queryKey: dealProductsKey(offer.deal_id),
      });
      void reloadReportingFigures(queryClient);
      setOpen(false);
    },
    onError: () => rereadOffer(queryClient, offer.id),
  });

  const errorMessage = mutation.isError
    ? problemMessageOf(mutation.error, t)
    : null;

  return (
    <>
      <Button
        variant="primary"
        data-testid="accept-offer"
        onClick={() => setOpen(true)}
      >
        <Check aria-hidden /> {t("offer.accept")}
      </Button>
      <ConfirmModal
        open={open}
        onClose={() => {
          mutation.reset();
          setOpen(false);
        }}
        title={t("offer.acceptConfirm")}
        confirmLabel={t("offer.accept")}
        onConfirm={() => mutation.mutate()}
        pending={mutation.isPending}
        error={errorMessage}
      >
        <p className="t-body">{t("offer.acceptBody")}</p>
      </ConfirmModal>
    </>
  );
}

// Reject never touches the deal's amount, but the deal screen reads this
// offer's status through the ["deal", id] and ["deal-offers", id] caches that
// accept invalidates. So reject invalidates them too, on the status change
// alone. The optional reason is a plain text field, not a bespoke form, since
// the action moves no money. It still uses the shared ConfirmModal for the
// disable-while-pending and inline-error handling Send and Accept get. It has
// no `tier`, because rejecting is not a confirm-first 🟡 operation.
export function RejectOfferAction({ offer }: Readonly<{ offer: Offer }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");

  const mutation = useMutation({
    mutationFn: async () => {
      return unwrap(
        await api.POST("/offers/{id}/reject", {
          params: {
            path: { id: offer.id },
            ...ifMatch(requireVersion(offer.version)),
          },
          body: { reason: reason || null },
        }),
      );
    },
    onSuccess: (data) => {
      queryClient.setQueryData(["offer", offer.id], data);
      queryClient.invalidateQueries({ queryKey: ["deal", offer.deal_id] });
      queryClient.invalidateQueries({
        queryKey: ["deal-offers", offer.deal_id],
      });
      setOpen(false);
      setReason("");
    },
    onError: () => rereadOffer(queryClient, offer.id),
  });

  const errorMessage = mutation.isError
    ? problemMessageOf(mutation.error, t)
    : null;

  return (
    <>
      <Button
        variant="danger"
        data-testid="reject-offer"
        onClick={() => setOpen(true)}
      >
        <X aria-hidden /> {t("offer.reject")}
      </Button>
      <ConfirmModal
        open={open}
        onClose={() => {
          mutation.reset();
          setOpen(false);
        }}
        title={t("offer.rejectConfirm")}
        confirmLabel={t("offer.reject")}
        confirmVariant="danger"
        onConfirm={() => mutation.mutate()}
        pending={mutation.isPending}
        error={errorMessage}
      >
        <Field label={t("offer.rejectReason")}>
          {(control) => (
            <TextInput
              {...control}
              data-testid="reject-reason"
              value={reason}
              onChange={(event) => setReason(event.target.value)}
            />
          )}
        </Field>
      </ConfirmModal>
    </>
  );
}

// Regenerate a new draft revision from a sent offer. The 201 response is the
// only place the AI provenance notice and diff summary appear. Every later
// read of that offer returns them null. So the new draft's cache entry is
// seeded from this response before navigating, and OfferScreen's query skips
// its refetch on mount for an AI-generated draft (offers.tsx). The sent offer
// stays sent or superseded on the server, never deleted. So unlike Send,
// regenerate needs no confirm modal.
export function RegenerateOfferAction({ offer }: Readonly<{ offer: Offer }>) {
  const t = useT();
  const queryClient = useQueryClient();

  const { key, renew } = useIdempotencyKey();
  const mutation = useMutation({
    mutationFn: async () => {
      return unwrap(
        await api.POST("/offers/{id}/regenerate", {
          params: {
            path: { id: offer.id },
            header: { "Idempotency-Key": key },
          },
        }),
      );
    },
    onMutate: fromHere,
    onSuccess: (newDraft, _vars, from) => {
      renew();
      queryClient.setQueryData(["offer", newDraft.id], newDraft);
      // The server has just superseded this offer; going back must not find it sent.
      void rereadOffer(queryClient, offer.id);
      // The new draft revision is a new offer row on the same deal. The deal's
      // offers panel (deals.overview.tsx) must see it too, as AcceptOfferAction
      // invalidates after its own change.
      queryClient.invalidateQueries({
        queryKey: ["deal-offers", newDraft.deal_id],
      });
      from.navigate({ screen: "offers", id: newDraft.id });
    },
    onError: () => rereadOffer(queryClient, offer.id),
  });

  const errorMessage = mutation.isError
    ? problemMessageOf(mutation.error, t)
    : null;

  return (
    <>
      <Button
        data-testid="regenerate-offer"
        pending={mutation.isPending}
        onClick={() => mutation.mutate()}
      >
        <RefreshCw aria-hidden /> {t("offer.regenerate")}
      </Button>
      {errorMessage && <ErrorLine>{errorMessage}</ErrorLine>}
    </>
  );
}

// Only a live draft takes edits; an archived one is read like a sent one.
export function isOpenDraft(offer: Offer): boolean {
  return offer.status === "draft" && offer.archived_at == null;
}

// Discarding archives the draft: it leaves the deal's offers and the reader
// goes back to the deal, since nothing on this page can change any more.
export function DiscardOfferAction({ offer }: Readonly<{ offer: Offer }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);

  const mutation = useMutation({
    mutationFn: async (version: number) =>
      unwrap(
        await api.DELETE("/offers/{id}", {
          params: { path: { id: offer.id }, ...ifMatch(version) },
        }),
      ),
    onMutate: fromHere,
    onSuccess: (data, _version, from) => {
      queryClient.setQueryData(["offer", offer.id], data);
      void queryClient.invalidateQueries({
        queryKey: ["deal-offers", offer.deal_id],
      });
      setOpen(false);
      from.closeOnto({ screen: "deals", id: offer.deal_id });
    },
    onError: () => rereadOffer(queryClient, offer.id),
  });

  return (
    <>
      <Button data-testid="discard-offer" onClick={() => setOpen(true)}>
        <Trash2 aria-hidden /> {t("offer.discard")}
      </Button>
      <ConfirmModal
        open={open}
        onClose={() => setOpen(false)}
        title={t("offer.discardConfirm")}
        confirmLabel={t("offer.discard")}
        confirmVariant="danger"
        onConfirm={() => mutation.mutate(requireVersion(offer.version))}
        pending={mutation.isPending}
        error={mutation.isError ? problemMessageOf(mutation.error, t) : null}
      >
        <p className="t-body">{t("offer.discardBody")}</p>
      </ConfirmModal>
    </>
  );
}

// Render the offer's branded PDF. A 501 means the deployment has no blobstore
// wired, as the attachments seam does when unwired. That is an outcome, not an
// error. It is read off the raw `response.status` before the `error` branch,
// so it never reaches throwProblem. Every other response takes that path.
// On 200 the full Offer seeds the cache, so the link reads off the `offer`.
export function RenderOfferPdfAction({ offer }: Readonly<{ offer: Offer }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const pdfHref = `${globalThis.location.origin}/v1/offers/${offer.id}/pdf`;

  const mutation = useMutation({
    mutationFn: async () => {
      const { data, error, response } = await api.POST("/offers/{id}/render", {
        params: { path: { id: offer.id } },
      });
      if (response.status === 501) {
        return { available: false as const };
      }
      if (error) {
        throwProblem(error);
      }
      return { available: true as const, offer: data };
    },
    onSuccess: (result) => {
      if (result.available) {
        queryClient.setQueryData(["offer", offer.id], result.offer);
      }
    },
  });

  const unavailable = mutation.data?.available === false;
  const errorMessage = mutation.isError
    ? problemMessageOf(mutation.error, t)
    : null;

  return (
    <Panel
      title={t("offer.renderPdf")}
      actions={
        <>
          <Button
            data-testid="render-pdf"
            pending={mutation.isPending}
            onClick={() => mutation.mutate()}
          >
            {t("offer.renderPdf")}
          </Button>
          {offer.pdf_asset_ref && (
            <a
              href={pdfHref}
              target="_blank"
              rel="noreferrer"
              data-testid="pdf-link"
            >
              {t("offer.viewPdf")}
            </a>
          )}
        </>
      }
    >
      {/* A body only when there is something to report: an empty one is a
          padded band saying nothing. */}
      {(unavailable || errorMessage) && (
        <PanelBody>
          {unavailable && (
            <p data-testid="pdf-unavailable">{t("offer.pdfUnavailable")}</p>
          )}
          {errorMessage && <ErrorLine>{errorMessage}</ErrorLine>}
        </PanelBody>
      )}
    </Panel>
  );
}
