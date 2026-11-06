// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useQuery } from "@tanstack/react-query";
import { Button } from "../../design-system/atoms";
import { ThreadState } from "../../design-system/emaildetail";
import {
  emailDetailKey,
  readEmailPresentation,
} from "../../design-system/emailmessage";
import { Panel, PanelBody } from "../../design-system/panel";
import { formatActivityWhen } from "../../format/activitywhen";
import { useLocale, useT } from "../../i18n";
import { useOpenSource } from "../../lib/opensource";
import { useViewerId } from "../../lib/session";
import { useViewerZone } from "../../lib/viewerzone";

/**
 * The messages behind a row, read through the email drawer's own thread
 * view. The newest message is open; older ones fold to one line each.
 */
export function ConversationPanel({
  activityId,
}: Readonly<{ activityId: string }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const viewerUserId = useViewerId();
  const openSource = useOpenSource();
  // A message's content is an authorization result, so it is asked for on
  // every open and never kept, as the email drawer asks.
  const read = useQuery({
    queryKey: emailDetailKey(activityId),
    queryFn: () => readEmailPresentation(activityId),
    staleTime: 0,
    gcTime: 0,
  });
  return (
    <Panel
      title={t("worklist.pane.conversation")}
      titleAction={
        openSource && (
          <Button variant="ghost" onClick={() => openSource(activityId)}>
            {t("worklist.pane.openEmail")}
          </Button>
        )
      }
    >
      <PanelBody>
        <ThreadState
          read={read}
          formatWhen={(at, precision) =>
            formatActivityWhen(at, precision, locale, zone)
          }
          viewerUserId={viewerUserId}
        />
      </PanelBody>
    </Panel>
  );
}
