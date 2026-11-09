// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import {
  useInfiniteQuery,
  useMutation,
  useQueryClient,
} from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Button } from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { Field, Textarea } from "../../design-system/field";
import { Heading } from "../../design-system/heading";
import { formatDateAbbrev } from "../../format/format";
import { sendingCompanyScope } from "../../format/sendingcompany";
import { useLocale, useT } from "../../i18n";
import { communicationContextLabel } from "../../lib/compose-context";
import { problemMessageOf, unwrap } from "../../lib/problem";
import { useViewerZone } from "../../lib/viewerzone";

type StandingOverride = components["schemas"]["StandingOverride"];

export function StandingExceptions({
  contactId,
}: Readonly<{ contactId: string }>) {
  const t = useT();
  const list = useInfiniteQuery({
    queryKey: ["contactStandingOverrides", contactId],
    initialPageParam: "",
    queryFn: async ({ pageParam, signal }) =>
      unwrap(
        await api.GET("/contacts/{id}/consent/allow", {
          params: {
            path: { id: contactId },
            query: { limit: 50, cursor: pageParam || undefined },
          },
          signal,
        }),
      ),
    getNextPageParam: (last) =>
      last.page.has_more ? (last.page.next_cursor ?? undefined) : undefined,
  });
  const rows = list.data?.pages.flatMap((page) => page.data) ?? [];
  if (!list.isError && rows.length === 0) return null;
  return (
    <section aria-label={t("consent.exceptionsTitle")}>
      <Heading as="h3" size="xsmall">
        {t("consent.exceptionsTitle")}
      </Heading>
      {rows.length > 0 && (
        <p className="t-caption">{t("consent.exceptionsScope")}</p>
      )}
      {rows.map((row) => (
        <ExceptionRow key={row.override_id} contactId={contactId} row={row} />
      ))}
      {list.isError && (
        <p role="alert">
          {problemMessageOf(list.error, t)}{" "}
          <Button variant="link" onClick={() => list.refetch()}>
            {t("common.retry")}
          </Button>
        </p>
      )}
      {list.hasNextPage && (
        <Button
          variant="link"
          pending={list.isFetchingNextPage}
          onClick={() => list.fetchNextPage()}
        >
          {t("list.loadMore")}
        </Button>
      )}
    </section>
  );
}

function ExceptionRow({
  contactId,
  row,
}: Readonly<{ contactId: string; row: StandingOverride }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const category =
    communicationContextLabel(row.category, t) ??
    t("consent.exceptionOtherPurpose");
  const date = row.recorded_at
    ? formatDateAbbrev(row.recorded_at, locale, zone)
    : undefined;
  return (
    <div>
      <p>{category}</p>
      <p className="t-caption">
        {sendingCompanyScope(row.sending_entity_id, row.sending_entity_name, t)}
      </p>
      <p className="t-caption">
        {date
          ? row.recorded_by_name
            ? t("consent.stopRecordedBy", { name: row.recorded_by_name, date })
            : t("consent.stopRecordedByColleague", { date })
          : t("consent.exceptionUnknownOrigin")}
      </p>
      <p className="t-caption">{row.reason}</p>
      {row.may_revoke && <RevokeException contactId={contactId} row={row} />}
    </div>
  );
}

function RevokeException({
  contactId,
  row,
}: Readonly<{ contactId: string; row: StandingOverride }>) {
  const t = useT();
  const client = useQueryClient();
  const [asking, setAsking] = useState(false);
  const [reason, setReason] = useState("");
  const revoke = useMutation({
    mutationFn: async (words: string) =>
      unwrap(
        await api.POST("/contacts/{id}/consent/allow/{overrideId}/revoke", {
          params: { path: { id: contactId, overrideId: row.override_id } },
          body: { reason: words },
        }),
      ),
    onSuccess: () => {
      setAsking(false);
      setReason("");
      for (const key of ["contactStandingOverrides", "contactConsentGuard"])
        client.invalidateQueries({ queryKey: [key, contactId] });
    },
  });
  return (
    <>
      <Button variant="link" onClick={() => setAsking(true)}>
        {t("consent.exceptionRevoke")}
      </Button>
      <ConfirmModal
        open={asking}
        onClose={() => {
          setAsking(false);
          revoke.reset();
        }}
        title={t("consent.exceptionRevoke")}
        confirmLabel={t("consent.exceptionRevoke")}
        confirmVariant="danger"
        confirmDisabled={reason.trim() === ""}
        onConfirm={() => revoke.mutate(reason.trim())}
        pending={revoke.isPending}
        error={revoke.isError ? problemMessageOf(revoke.error, t) : null}
      >
        <p>{t("consent.exceptionRevokeBody")}</p>
        <p className="t-caption">{row.reason}</p>
        <Field label={t("consent.stopReason")} required>
          {(control) => (
            <Textarea
              {...control}
              value={reason}
              maxLength={500}
              onChange={(event) => setReason(event.target.value)}
            />
          )}
        </Field>
      </ConfirmModal>
    </>
  );
}
