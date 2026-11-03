// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { AppLink } from "../../design-system/applink";
import { EmptyState } from "../../design-system/atoms";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { useT } from "../../i18n";
import { analyticsSectionHref } from "../../lib/analytics.address";
import { useUrlParams } from "../../lib/urlstate";
import { OVERVIEW_DIAL } from "./analytics.overview.choice";
import { useProductOptions } from "./reporting.dimensions";

/**
 * Product sales: one entry per live product, each opening Sales review
 * narrowed to that product. No report is written and nothing is set up; a
 * product report is the company report read for one product.
 */
export function ProductReports() {
  const t = useT();
  const [params] = useUrlParams();
  const products = useProductOptions();
  return (
    <Panel title={t("reporting.productReports")}>
      <PanelBody>
        <p className="t-caption">{t("reporting.productReportsPurpose")}</p>
        <QueryGate
          query={products}
          pendingLabel={t("reporting.productReports")}
        >
          {(options) => (
            <>
              {options.length === 0 && (
                <EmptyState>{t("reporting.noProducts")}</EmptyState>
              )}
              {options.map((product) => (
                <PanelRow key={product.value}>
                  <AppLink
                    className="btn btn-ghost"
                    href={analyticsSectionHref(
                      "overview",
                      productDials(params, product.value),
                    )}
                  >
                    {product.label}
                  </AppLink>
                </PanelRow>
              ))}
            </>
          )}
        </QueryGate>
      </PanelBody>
    </Panel>
  );
}

// Sales review for one product: the reader's period, scope and pipeline stay,
// and the market dials of an earlier question are cleared with it.
function productDials(
  params: ReadonlyMap<string, string>,
  productId: string,
): Map<string, string> {
  const next = new Map(params);
  for (const dial of ["country", "industry", "basis", "view"])
    next.delete(dial);
  next.set("product", productId);
  next.set(OVERVIEW_DIAL, "sales");
  return next;
}
