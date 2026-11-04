// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Trash2 } from "lucide-react";
import { useState } from "react";
import { api } from "../../../api/client";
import type { components } from "../../../api/schema";
import { Badge, Button } from "../../../design-system/atoms";
import { ConfirmModal } from "../../../design-system/confirmmodal";
import { ErrorLine } from "../../../design-system/errorline";
import { IconAction } from "../../../design-system/iconaction";
import { MoneyInput } from "../../../design-system/moneyinput";
import {
  OverviewCard,
  OverviewList,
  OverviewRow,
  OverviewText,
} from "../../../design-system/overviewcard";
import {
  RecordPicker,
  type RecordPickerCandidate,
} from "../../../design-system/recordpicker";
import { formatMoneyOrAbsent } from "../../../format/format";
import { useLocale, useT } from "../../../i18n";
import { problemMessageOf, problemStatus, unwrap } from "../../../lib/problem";
import { REPORTING_EVALUATION_KEY } from "../../../lib/reportingfigures";
import { searchProductCandidates } from "../products";

type DealProduct = components["schemas"]["DealProduct"];

/** One product the editor holds, with the amount in the deal's currency. */
type Allocation = {
  productId: string;
  name: string;
  amountMinor: number | null;
};

export const dealProductsKey = (dealId: string) =>
  ["deal-products", dealId] as const;

/**
 * The products a deal sells, with the value allocated to each. The accepted
 * offer fills them; a writer may set them by hand, and a hand-set list then
 * stands until changed again. A deal with no accepted offer has no product
 * until somebody sets one, and product reports leave it out.
 */
export function DealProductsCard({
  dealId,
  currency,
  refusedReasonId,
}: Readonly<{
  dealId: string;
  // Amounts are written in the deal's currency. A deal nobody priced takes
  // products without amounts.
  currency: string | null;
  // The page's sentence about why the deal takes no changes, when it does not.
  refusedReasonId?: string;
}>) {
  const t = useT();
  const { locale } = useLocale();
  const [editing, setEditing] = useState(false);
  const query = useQuery({
    queryKey: dealProductsKey(dealId),
    queryFn: async (): Promise<DealProduct[]> =>
      unwrap(
        await api.GET("/deals/{id}/products", {
          params: { path: { id: dealId } },
        }),
        t,
      ).data,
  });
  const title = t("deal.products.title");
  if (query.isError) {
    return (
      <OverviewCard title={title}>
        {problemStatus(query.error) === 403 ? (
          <OverviewText>{t("deal.products.denied")}</OverviewText>
        ) : (
          <ErrorLine error={query.error} standing />
        )}
      </OverviewCard>
    );
  }
  if (!query.data) {
    return <OverviewCard title={title} />;
  }
  const rows = query.data;
  return (
    <>
      <OverviewCard
        title={title}
        action={
          <Button
            variant="link"
            reasonId={refusedReasonId}
            onClick={() => setEditing(true)}
          >
            {t("deal.products.edit")}
          </Button>
        }
        empty={t("deal.products.empty")}
      >
        {rows.length > 0 && (
          <OverviewList>
            {rows.map((row) => (
              <OverviewRow
                key={row.product_id}
                title={row.product_name}
                sub={formatMoneyOrAbsent(
                  row.amount_minor,
                  row.currency,
                  locale,
                )}
                right={
                  <Badge>
                    {row.source === "offer"
                      ? t("deal.products.fromOffer")
                      : t("deal.products.setByHand")}
                  </Badge>
                }
              />
            ))}
          </OverviewList>
        )}
      </OverviewCard>
      {/* Beside the card, not inside it: the card draws its empty words only
          while it holds no body. */}
      {editing && (
        <DealProductsEditor
          dealId={dealId}
          currency={currency}
          current={rows}
          onClose={() => setEditing(false)}
        />
      )}
    </>
  );
}

/** Replaces the deal's products with the list the writer builds here. */
export function DealProductsEditor({
  dealId,
  currency,
  current,
  onClose,
}: Readonly<{
  dealId: string;
  currency: string | null;
  current: readonly DealProduct[];
  onClose: () => void;
}>) {
  const t = useT();
  const client = useQueryClient();
  const [list, setList] = useState<Allocation[]>(() =>
    current.map((row) => ({
      productId: row.product_id,
      name: row.product_name,
      amountMinor: row.amount_minor ?? null,
    })),
  );
  const replace = useMutation({
    mutationFn: async (next: readonly Allocation[]) =>
      unwrap(
        await api.PUT("/deals/{id}/products", {
          params: { path: { id: dealId } },
          body: {
            products: next.map((row) => ({
              product_id: row.productId,
              amount_minor: currency ? row.amountMinor : null,
            })),
          },
        }),
        t,
      ).data,
    onSuccess: (saved) => {
      client.setQueryData(dealProductsKey(dealId), saved);
      // The commercial mix and product bookings read these allocations.
      void client.invalidateQueries({ queryKey: REPORTING_EVALUATION_KEY });
      onClose();
    },
  });
  const fromOffer = current.some((row) => row.source === "offer");
  const listed = new Set(list.map((row) => row.productId));
  const add = (candidate: RecordPickerCandidate) =>
    setList((rows) => [
      ...rows,
      { productId: candidate.id, name: candidate.name, amountMinor: null },
    ]);
  const update = (productId: string, amountMinor: number | null) =>
    setList((rows) =>
      rows.map((row) =>
        row.productId === productId ? { ...row, amountMinor } : row,
      ),
    );
  const remove = (productId: string) =>
    setList((rows) => rows.filter((row) => row.productId !== productId));
  return (
    <ConfirmModal
      open
      intent="form"
      onClose={onClose}
      title={t("deal.products.editTitle")}
      confirmLabel={t("deal.products.save")}
      pending={replace.isPending}
      error={replace.error ? problemMessageOf(replace.error, t) : null}
      onConfirm={() => replace.mutate(list)}
    >
      <p className="t-body">
        {fromOffer
          ? t("deal.products.replacesOffer")
          : t("deal.products.intro")}
      </p>
      {list.length === 0 && (
        <p className="t-caption">{t("deal.products.emptyTakesOffer")}</p>
      )}
      {list.length > 0 && (
        <ul className="deal-products-edit">
          {list.map((row) => (
            <li key={row.productId} className="deal-products-edit-row">
              <span className="t-body">{row.name}</span>
              {currency && (
                <MoneyInput
                  aria-label={t("deal.products.amount", { name: row.name })}
                  currency={currency}
                  valueMinor={row.amountMinor ?? 0}
                  blankWhenZero
                  onChangeMinor={(minor) => update(row.productId, minor)}
                  onClear={() => update(row.productId, null)}
                />
              )}
              <IconAction
                icon={<Trash2 />}
                label={t("deal.products.remove", { name: row.name })}
                onClick={() => remove(row.productId)}
              />
            </li>
          ))}
        </ul>
      )}
      <RecordPicker
        label={t("deal.products.add")}
        searchTargets={searchProductCandidates}
        unavailableReason={(candidate) =>
          listed.has(candidate.id)
            ? t("deal.products.alreadyListed")
            : undefined
        }
        onPick={add}
      />
    </ConfirmModal>
  );
}
