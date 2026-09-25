// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useId, useRef, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Button, EmptyState } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Heading } from "../design-system/heading";
import { Modal } from "../design-system/modal";
import { Panel, PanelBody, PanelIntro, PanelRow } from "../design-system/panel";
import { formatDate } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { useAnalyticsContext } from "./analytics.context";
import { problemMessageOf, QueryStates, throwProblem } from "./common";

// The forecast links a reader has issued and that still open, listed so any of
// them can be closed after the dialog that showed it is gone. The server
// answers only the caller's own links and never their tokens: a row here can
// be ended, not copied again.

type OpenShare = components["schemas"]["ForecastShare"];

// The caller's open links, under ONE key: issuing a share, closing it from the
// dialog that issued it and closing it from the list all change this answer.
export const OPEN_SHARES_KEY = ["forecast-shares"] as const;

const KIND_LABELS: Record<OpenShare["kind"], MessageKey> = {
  live: "analytics.share.liveLabel",
  snapshot: "analytics.share.snapshotLabel",
};

// A population the scope picker cannot name: a team or colleague this reader
// no longer measures. A uuid names nothing a reader can act on, so the kind.
const POPULATION_FALLBACK: Record<OpenShare["scope_kind"], MessageKey> = {
  workspace: "analytics.share.populationCompany",
  team: "analytics.share.populationTeam",
  owner: "analytics.share.populationOwner",
};

export async function closeShare(id: string): Promise<void> {
  const { error } = await api.DELETE("/forecast/shares/{id}", {
    params: { path: { id } },
  });
  if (error) {
    throwProblem(error);
  }
}

// `canClose` is the write half: a read seat may list its links and is refused
// any close, so it is shown the rows without the verb.
export function SharedLinksButton({
  canClose,
}: Readonly<{ canClose: boolean }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const headingId = useId();
  return (
    <>
      <Button onClick={() => setOpen(true)}>
        {t("analytics.share.listOpen")}
      </Button>
      <Modal
        open={open}
        onClose={() => setOpen(false)}
        labelledBy={headingId}
        placement="right"
      >
        <Heading size="large" id={headingId} className="t-h2 modal-title">
          {t("analytics.share.listTitle")}
        </Heading>
        <OpenShareList headingId={headingId} canClose={canClose} />
      </Modal>
    </>
  );
}

// The links this reader issued that still open, each with Close link behind
// one shared confirmation. The drawer stays open across a close, so a reader
// ending three links does not reopen it three times.
function OpenShareList({
  headingId,
  canClose,
}: Readonly<{ headingId: string; canClose: boolean }>) {
  const t = useT();
  const queryClient = useQueryClient();
  const shares = useQuery({
    queryKey: OPEN_SHARES_KEY,
    queryFn: async () => {
      const { data, error } = await api.GET("/forecast/shares", {});
      if (error) {
        throwProblem(error);
      }
      return data.data;
    },
  });
  // The same read the scope picker draws from, so a link names its
  // population in the words the picker used for it.
  const scopes = useAnalyticsContext().data?.allowed_scopes ?? [];
  const [closing, setClosing] = useState<OpenShare | null>(null);
  const opener = useRef<{ button: HTMLElement; index: number } | null>(null);
  const close = useMutation({
    mutationFn: closeShare,
    // The confirmation stays pending until the list has re-read, so it never
    // closes onto the row it just ended.
    onSuccess: async () => {
      await queryClient.invalidateQueries({ queryKey: OPEN_SHARES_KEY });
      setClosing(null);
    },
    // A refusal can mean the link is already gone (expired, or closed from
    // another tab), so the list re-reads rather than keep a dead row.
    onError: () => queryClient.invalidateQueries({ queryKey: OPEN_SHARES_KEY }),
  });
  // After a close the opener's row is gone: the link now in its place takes
  // focus, else the one above it, else the drawer, never the page under it.
  const returnFocus = () => {
    if (!opener.current || opener.current.button.isConnected) {
      return opener.current?.button ?? null;
    }
    const drawer = document
      .getElementById(headingId)
      ?.closest<HTMLElement>('[role="dialog"]');
    const verbs = [
      ...(drawer?.querySelectorAll<HTMLElement>(".meta-row-action button") ??
        []),
    ];
    const { index } = opener.current;
    return verbs[index] ?? verbs[index - 1] ?? drawer ?? null;
  };
  const rows = shares.data ?? [];

  return (
    <Panel>
      <PanelBody>
        <PanelIntro>{t("analytics.share.listIntro")}</PanelIntro>
      </PanelBody>
      <QueryStates
        query={shares}
        pendingLabel={t("analytics.share.listTitle")}
        pendingLines={3}
      >
        {rows.length === 0 ? (
          <EmptyState>{t("analytics.share.listEmpty")}</EmptyState>
        ) : (
          rows.map((share, index) => (
            <OpenShareRow
              key={share.id}
              share={share}
              population={
                scopes.find(
                  (scope) =>
                    scope.kind === share.scope_kind &&
                    scope.id === share.scope_id,
                )?.label ?? t(POPULATION_FALLBACK[share.scope_kind])
              }
              onClose={
                canClose
                  ? (button) => {
                      opener.current = { button, index };
                      setClosing(share);
                    }
                  : undefined
              }
            />
          ))
        )}
      </QueryStates>
      <ConfirmModal
        open={closing !== null}
        onClose={() => {
          setClosing(null);
          close.reset();
        }}
        title={t("analytics.share.closeTitle")}
        confirmLabel={t("analytics.share.revoke")}
        confirmVariant="danger"
        returnFocusTo={returnFocus}
        onConfirm={() => {
          if (closing) {
            close.mutate(closing.id);
          }
        }}
        pending={close.isPending}
        error={close.error ? problemMessageOf(close.error, t) : undefined}
      >
        <p>{t("analytics.share.closeBody")}</p>
      </ConfirmModal>
    </Panel>
  );
}

function OpenShareRow({
  share,
  population,
  onClose,
}: Readonly<{
  share: OpenShare;
  population: string;
  // Absent for a seat that may not close: the row stays, the verb does not.
  onClose?: (button: HTMLElement) => void;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const populationId = useId();
  return (
    <PanelRow className="meta-row">
      <span className="meta-row-line t-caption">
        <span>{t(KIND_LABELS[share.kind])}</span>
        <span>
          {t("analytics.share.listCreated", {
            date: formatDate(share.created_at, locale, zone),
          })}
        </span>
        <span>
          {t("analytics.share.listExpires", {
            date: formatDate(share.expires_at, locale, zone),
          })}
        </span>
      </span>
      <span className="meta-row-entry t-body" id={populationId}>
        {population}
      </span>
      {onClose && (
        <span className="meta-row-action">
          {/* Every row's verb reads "Close link"; the population it describes
              is what tells a screen-reader user which link it ends. */}
          <Button
            variant="danger"
            aria-describedby={populationId}
            onClick={(event) => onClose(event.currentTarget)}
          >
            {t("analytics.share.revoke")}
          </Button>
        </span>
      )}
    </PanelRow>
  );
}
