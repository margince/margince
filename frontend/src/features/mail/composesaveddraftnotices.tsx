// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { ErrorLine } from "../../design-system/errorline";
import { formatNumber } from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import type { SavedDraft } from "./composesaveddraft";

// What the composer says about its held draft: put back with any file that
// could not return, or moved by another window.
export function SavedDraftNotices({ draft }: Readonly<{ draft: SavedDraft }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (draft.changedElsewhere) {
    return (
      <Callout
        tone="warning"
        kind="outcome"
        title={t("compose.savedDraftChangedTitle")}
        actions={
          draft.held ? (
            <Button onClick={draft.loadSaved}>
              {t("compose.savedDraftLoad")}
            </Button>
          ) : undefined
        }
      >
        {t(
          draft.held
            ? "compose.savedDraftChangedBody"
            : "compose.savedDraftGoneBody",
        )}
      </Callout>
    );
  }
  if (!draft.restored) return null;
  // An agent's words are marked as an agent's until the reader saves over
  // them, so they are read before they are sent.
  const byAgent = draft.held?.agent_drafted === true;
  const lost = draft.held?.attachments_unavailable ?? 0;
  return (
    <div
      className={byAgent ? "compose-strip compose-strip-ai" : "compose-strip"}
      role="status"
    >
      <div className="compose-strip-text">
        <p>
          {t(
            byAgent
              ? "compose.savedDraftByAgent"
              : "compose.savedDraftRestored",
          )}
        </p>
        {lost > 0 && (
          <p>
            {plural("compose.savedDraftFilesUnavailable", lost, {
              count: formatNumber(lost, locale),
            })}
          </p>
        )}
      </div>
      <Button variant="link" onClick={draft.remove} pending={draft.removing}>
        {t("compose.savedDraftRemove")}
      </Button>
    </div>
  );
}

/** A refused write of the saved draft, with the way to try it again now. */
export function SavedDraftFailure({ draft }: Readonly<{ draft: SavedDraft }>) {
  const t = useT();
  if (!draft.error) return null;
  return (
    <ErrorLine
      actions={
        draft.retry && (
          <Button variant="link" onClick={draft.retry}>
            {t("common.retry")}
          </Button>
        )
      }
    >
      {draft.error}
    </ErrorLine>
  );
}
