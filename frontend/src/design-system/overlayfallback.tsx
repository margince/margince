// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useT } from "../i18n";
import { EmptyState } from "./atoms";

/**
 * OverlayFallback replaces a 360 page when the workspace reads from an
 * incumbent mirror. The 360 read answers a refusal to assemble rather than a
 * failure, so the page falls back instead of showing an error — which is why
 * this is an EmptyState and not an error surface: nothing went wrong, there is
 * simply nothing here to assemble from.
 */
export function OverlayFallback() {
  const t = useT();
  return <EmptyState>{t("co.overlayFallback")}</EmptyState>;
}
