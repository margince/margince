// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Archiving a field asks first, and says which Live Lists filter on it. They
// keep working on the values already stored, so nothing blocks the archive;
// the reader learns which lists will ask their stewards for a new clause.

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { ConfirmModal } from "../design-system/confirmmodal";
import { SurfaceState } from "../design-system/surfacestate";
import { formatNumber } from "../format/format";
import { useLocale, usePlural, useT } from "../i18n";
import { problemMessageOf, throwProblem } from "./common";
import { LISTS_KEY } from "./lists.queries";

type CustomField = components["schemas"]["CustomField"];
type LiveLists = components["schemas"]["CustomFieldLiveLists"];

export function RetireFieldConfirm({
  field,
  onClose,
  onRetired,
}: Readonly<{
  field: CustomField | null;
  onClose: () => void;
  onRetired: (field: CustomField) => void;
}>) {
  const t = useT();
  const queryClient = useQueryClient();
  const retire = useMutation({
    mutationFn: async (target: CustomField) => {
      const { data, error } = await api.POST("/custom-fields/{id}/retire", {
        params: { path: { id: target.id } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (_data, target) => {
      // A list on the field now reports it, so every list read is stale.
      queryClient.invalidateQueries({ queryKey: [LISTS_KEY] });
      onRetired(target);
    },
  });
  return (
    <ConfirmModal
      open={field !== null}
      onClose={() => {
        retire.reset();
        onClose();
      }}
      tier="confirm"
      title={t("cf.retire.title", { label: field?.label ?? "" })}
      confirmLabel={t("cf.archive")}
      pending={retire.isPending}
      error={retire.isError ? problemMessageOf(retire.error, t) : null}
      onConfirm={() => {
        if (field) {
          retire.mutate(field);
        }
      }}
    >
      <p>{t("cf.retire.body")}</p>
      {field && <FieldLiveLists fieldId={field.id} />}
    </ConfirmModal>
  );
}

function FieldLiveLists({ fieldId }: Readonly<{ fieldId: string }>) {
  const t = useT();
  const lists = useQuery({
    // Under the lists key, so every list write that can move the answer
    // (sharing, archive, a new Live List) refetches it.
    queryKey: [LISTS_KEY, "field-use", fieldId],
    queryFn: async () => {
      const { data, error } = await api.GET("/custom-fields/{id}/lists", {
        params: { path: { id: fieldId } },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
  });
  const found = lists.data;
  const none =
    found !== undefined && found.lists.length === 0 && found.unseen_count === 0;
  return (
    <SurfaceState
      state={
        lists.isPending
          ? "loading"
          : lists.isError
            ? "unavailable"
            : none
              ? "empty"
              : "ready"
      }
      emptyLabel={t("cf.retire.noLists")}
      loadingLabel={t("cf.retire.checking")}
      loadingLines={2}
    >
      {found && <LiveListNames found={found} />}
    </SurfaceState>
  );
}

function LiveListNames({ found }: Readonly<{ found: LiveLists }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  return (
    <>
      {found.lists.length > 0 && (
        <>
          <p>{t("cf.retire.lists")}</p>
          <ul>
            {found.lists.map((list) => (
              <li key={list.id}>{list.name}</li>
            ))}
          </ul>
        </>
      )}
      {found.unseen_count > 0 && (
        <p className="t-caption">
          {plural("cf.retire.unseen", found.unseen_count, {
            count: formatNumber(found.unseen_count, locale),
          })}
        </p>
      )}
    </>
  );
}
