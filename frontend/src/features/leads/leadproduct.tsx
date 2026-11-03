// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { X } from "lucide-react";
import { useState } from "react";
import type { components } from "../../api/schema";
import {
  searchProductCandidates,
  useProductName,
} from "../../components/productsearch";
import { ErrorLine } from "../../design-system/errorline";
import { IconAction } from "../../design-system/iconaction";
import { ListPopover } from "../../design-system/listpopover";
import { useT } from "../../i18n";
import type { LeadWriter } from "./leads";

type Lead = components["schemas"]["Lead"];

/**
 * The product a lead is interested in, with the verbs that name, change or
 * clear it. Product reports count the lead under it, so a writer sets it here
 * when the lead did not arrive with one.
 */
export function LeadProductLine({
  lead,
  writer,
  reasonId,
}: Readonly<{ lead: Lead; writer: LeadWriter; reasonId?: string }>) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const name = useProductName(lead.product_id);
  const label = lead.product_id
    ? t("lead.product.change")
    : t("lead.product.attach");
  // This write's own outcome, not any write on the lead: the mutation is
  // shared with the Details card, as LeadProjectAttachButton explains.
  const thisWrite =
    writer.patch.variables?.body !== undefined &&
    "product_id" in writer.patch.variables.body;
  const failed = writer.patch.isError && thisWrite;
  return (
    <div className="card-actions">
      {lead.product_id && (
        <span className="t-body">{name.data ?? t("lead.product.loading")}</span>
      )}
      <ListPopover
        label={label}
        title={label}
        searchLabel={t("lead.product.search")}
        reasonId={reasonId}
        open={open}
        onOpenChange={(next) => {
          if (next || !writer.patch.isPending) {
            setOpen(next);
          }
        }}
        search={searchProductCandidates}
        selected={lead.product_id ?? undefined}
        // The popover closes on the pick; a refused save is said on the line.
        onPick={(option, done) => {
          done();
          if (option.id !== lead.product_id) {
            writer.save({ product_id: option.id });
          }
        }}
        pending={writer.patch.isPending}
      />
      {lead.product_id && (
        <IconAction
          icon={<X />}
          label={t("lead.product.clear")}
          reasonId={reasonId}
          pending={writer.patch.isPending && thisWrite}
          onClick={() => writer.save({ product_id: null })}
        />
      )}
      {failed && <ErrorLine error={writer.patch.error} inline />}
    </div>
  );
}
