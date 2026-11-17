// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import {
  searchProductCandidates,
  useProductName,
} from "../../components/productsearch";
import { Button } from "../../design-system/atoms";
import { ListPopover } from "../../design-system/listpopover";
import { useT } from "../../i18n";
import { problemMessageOf } from "../../lib/problem";
import type { LeadWriter } from "./leads";

type Lead = components["schemas"]["Lead"];

/** The product a lead is interested in, as its row reads it. */
export function LeadProductName({ lead }: Readonly<{ lead: Lead }>) {
  const t = useT();
  const name = useProductName(lead.product_id);
  if (!lead.product_id) {
    return t("field.unset");
  }
  return name.data ?? t("lead.product.loading");
}

/**
 * The search that names, changes or clears a lead's product, opened by the
 * details' Edit. Product reports count the lead under it, so a writer sets it
 * here when the lead did not arrive with one.
 */
export function LeadProductPicker({
  lead,
  writer,
  open,
  onClose,
}: Readonly<{
  lead: Lead;
  writer: LeadWriter;
  open: boolean;
  onClose: () => void;
}>) {
  const t = useT();
  const label = lead.product_id
    ? t("lead.product.change")
    : t("lead.product.attach");
  // This write's own outcome, not any write on the lead: the mutation is
  // shared with the rest of the page.
  const thisWrite =
    writer.patch.variables?.body !== undefined &&
    "product_id" in writer.patch.variables.body;
  const failed = writer.patch.isError && thisWrite;
  return (
    <ListPopover
      opener="caller"
      open={open}
      onClose={() => {
        if (!writer.patch.isPending) {
          onClose();
        }
      }}
      title={label}
      searchLabel={t("lead.product.search")}
      search={searchProductCandidates}
      selected={lead.product_id ?? undefined}
      onPick={(option, done) => {
        if (option.id === lead.product_id) {
          done();
          return;
        }
        writer.save({ product_id: option.id }, { onSuccess: done });
      }}
      pending={writer.patch.isPending}
      error={failed ? problemMessageOf(writer.patch.error, t) : undefined}
      footer={
        lead.product_id && (
          <Button
            variant="ghost"
            pending={writer.patch.isPending}
            onClick={() =>
              writer.save({ product_id: null }, { onSuccess: onClose })
            }
          >
            {t("lead.product.clear")}
          </Button>
        )
      }
    />
  );
}
