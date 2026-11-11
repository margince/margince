// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { useId, useState } from "react";
import type { components } from "../../api/schema";
import { Button, Modal, PendingBody } from "../../design-system/atoms";
import { ErrorLine } from "../../design-system/errorline";
import { Heading } from "../../design-system/heading";
import { useT } from "../../i18n";
import { useContactGraph } from "./contactgraph";
import { RoutesPanel } from "./contactroutes";
import { IntroDrawer } from "./introdrawer";
import type { IntroRequest } from "./introrequests";

type RouteCandidate = components["schemas"]["ContactGraphRouteCandidate"];

export function IntroReplacement({
  contactId,
  contactName,
  request,
  onClose,
}: Readonly<{
  contactId: string;
  contactName: string;
  request: IntroRequest;
  onClose: () => void;
}>) {
  const t = useT();
  const title = useId();
  const graph = useContactGraph(contactId);
  const [route, setRoute] = useState<RouteCandidate>();
  if (route) {
    return (
      <IntroDrawer
        contactId={contactId}
        contactName={contactName}
        route={route}
        replacement={request}
        open
        onClose={onClose}
      />
    );
  }
  return (
    <Modal intent="drawer-reading" open onClose={onClose} labelledBy={title}>
      <Heading id={title} size="large" className="modal-title">
        {t("contact.intro.replaceAction")}
      </Heading>
      <p>{t("contact.intro.replaceHint")}</p>
      {graph.isPending ? <PendingBody label={t("common.loading")} /> : null}
      {graph.isError ? (
        <ErrorLine
          error={graph.error}
          actions={
            <Button variant="link" onClick={() => graph.refetch()}>
              {t("common.retry")}
            </Button>
          }
        />
      ) : null}
      {graph.data ? <RoutesPanel graph={graph.data} onAsk={setRoute} /> : null}
      <div className="actions">
        <Button variant="ghost" onClick={onClose}>
          {t("contact.intro.cancel")}
        </Button>
      </div>
    </Modal>
  );
}
