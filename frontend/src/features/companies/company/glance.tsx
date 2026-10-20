// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { ReactNode } from "react";
import type { components } from "../../../api/schema";
import { Button } from "../../../design-system/atoms";
import { Panel, PanelBody } from "../../../design-system/panel";
import {
  SurfaceState,
  sectionState,
} from "../../../design-system/surfacestate";
import { useT } from "../../../i18n";
import { recordNamesIn } from "../company360";
import { CompanyRecentList } from "../companyrecent";
import "./glance.css";

type Company360 = components["schemas"]["Company360"];

// How many exchanges the overview shows. The History tab holds the rest.
const LATEST_LIMIT = 4;

/**
 * LatestSection is what happened on the account lately, newest first. The way
 * to the History tab sits beside its title.
 *
 * It asks `sectionState` the same question the chronicle does. A section this
 * reader may not see says so, rather than reading as an account nobody wrote to.
 */
export function LatestSection({
  view,
  loading,
  onOpenHistory,
  onOpenRecord,
  onOpenEmail,
  footer,
}: Readonly<{
  view?: Company360;
  loading: boolean;
  // What is new since the reader last looked, under the rows it counts.
  footer?: ReactNode;
  onOpenHistory?: () => void;
  onOpenRecord?: (entityType: string, entityId: string) => void;
  // The page's email drawer. The rows draw messages in full, so without this
  // the section shows a reader the message and refuses to open it.
  onOpenEmail?: (activityId: string) => void;
}>) {
  const t = useT();
  const logged = view?.activities?.data ?? [];
  const state = sectionState(
    view,
    "activities",
    Boolean(view?.activities),
    logged.length,
    loading,
  );
  return (
    <Panel
      className="co-latest"
      title={t("co.latest.title")}
      footer={footer}
      titleAction={
        onOpenHistory && (
          <Button variant="link" onClick={onOpenHistory}>
            {t("co.latest.all")}
          </Button>
        )
      }
    >
      <PanelBody>
        {state === "ready" ? (
          <CompanyRecentList
            activities={logged.slice(0, LATEST_LIMIT)}
            nameOf={recordNamesIn(view)}
            onOpenRecord={onOpenRecord}
            onOpenEmail={onOpenEmail}
          />
        ) : (
          <SurfaceState
            loadingLabel={t("co.recent.title")}
            state={state}
            emptyLabel={t("co.recent.empty")}
            emptyDetail={t("co.recent.emptyDetail")}
          >
            {null}
          </SurfaceState>
        )}
      </PanelBody>
    </Panel>
  );
}
