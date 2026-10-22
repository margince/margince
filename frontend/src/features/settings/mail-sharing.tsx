import { useMutation, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useId, useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import {
  MAIL_REVIEW_BOUNDS,
  MAIL_SYNC_BOUNDS,
  useCaptureSettings,
  useUpdateCaptureSettings,
} from "../../components/capture-settings";
import { Button } from "../../design-system/atoms";
import { Callout } from "../../design-system/callout";
import { NumberSettingRow } from "../../design-system/numbersetting";
import { Panel, PanelBody } from "../../design-system/panel";
import { PanelNotices } from "../../design-system/panelnotices";
import { QueryGate } from "../../design-system/querystates";
import { SettingList, SettingRow } from "../../design-system/settingrow";
import { Switch } from "../../design-system/switch";
import { useToast } from "../../design-system/toast";
import { lockReasonOr } from "../../format/settinglocks";
import { useT } from "../../i18n";
import { useCanWrite } from "../../lib/capability";
import { problemMessageOf, unwrap } from "../../lib/problem";

// The workspace mail-sharing posture: ON by default, captured mail is
// readable by every colleague who can see the contact — the thing that makes
// the pipeline shared. Switching it OFF holds every email captured from then
// on to its participants, which makes shared CRM work hard, so the change is
// a deliberate act: a switch plus a Save button plus a warning that says the
// cost out loud, never a silent instant toggle. Every role sees the posture;
// only admin/ops may change it (same gating as the auto-enrich card).

type BandState = Readonly<{
  notice: ReactNode;
  error: unknown;
  dirty: boolean;
}>;

function bandShows({ notice, error, dirty }: BandState): boolean {
  return notice !== null || error !== undefined || dirty;
}

/**
 * The band under the rows: what the posture costs, what the write refused, and
 * the verb that commits it.
 *
 * All three belong to the CARD rather than to a row — an unsaved flip is a
 * state of the whole card, and a notice squeezed into a row's right column
 * would read as that switch's own answer. One component because this card
 * carries two of them, and two inline copies of the shape had already begun to
 * disagree about which of their notices interrupts.
 */
function CommitBand({
  notice,
  error,
  dirty,
  pending,
  onSave,
}: BandState & Readonly<{ pending: boolean; onSave: () => void }>) {
  const t = useT();
  if (!bandShows({ notice, error, dirty })) return null;
  return (
    <div className="settings-panel-commit">
      {notice}
      {error !== undefined && (
        <Callout
          kind="outcome"
          tone="danger"
          title={t("mailSharing.saveFailed")}
        >
          {problemMessageOf(error, t)}
        </Callout>
      )}
      {dirty && (
        <Button variant="primary" disabled={pending} onClick={onSave}>
          {t("mailSharing.save")}
        </Button>
      )}
    </div>
  );
}

/**
 * The two numbers that pace mail: how often a mailbox is read, and how many held
 * threads one privacy review decides. Each commits on its own, like every
 * number row, because pacing withholds nothing from anybody.
 */
function PacingRows({
  settings,
  locked,
  notAdmin,
  describedBy,
  onPatch,
}: Readonly<{
  settings: components["schemas"]["CaptureSettings"];
  locked: boolean;
  notAdmin: string | undefined;
  describedBy: string | undefined;
  onPatch: (
    patch: components["schemas"]["UpdateCaptureSettingsRequest"],
  ) => void;
}>) {
  const t = useT();
  return (
    <>
      <NumberSettingRow
        label={t("captureMailSync.interval.label")}
        description={t("captureMailSync.interval.help")}
        testId="capture-mail-sync"
        value={settings.mail_sync_interval_seconds}
        {...MAIL_SYNC_BOUNDS.mail_sync_interval_seconds}
        refusal={t("captureMailSync.interval.refusal")}
        disabled={locked}
        reason={lockReasonOr(
          settings.locks,
          "capture.mail_sync_interval_seconds",
          t,
          notAdmin,
        )}
        describedBy={describedBy}
        onCommit={(next) => onPatch({ mail_sync_interval_seconds: next })}
      />
      <NumberSettingRow
        label={t("captureReview.passCap.label")}
        description={t("captureReview.passCap.help")}
        testId="capture-review-pass-cap"
        value={settings.confidentiality_pass_cap}
        {...MAIL_REVIEW_BOUNDS.confidentiality_pass_cap}
        refusal={t("captureReview.passCap.refusal")}
        disabled={locked}
        reason={lockReasonOr(
          settings.locks,
          "capture.confidentiality_pass_cap",
          t,
          notAdmin,
        )}
        describedBy={describedBy}
        onCommit={(next) => onPatch({ confidentiality_pass_cap: next })}
      />
    </>
  );
}

export function MailSharingCard() {
  const t = useT();
  const toast = useToast();
  const canManage = useCanWrite("capture_settings", "update");
  const query = useCaptureSettings();
  const sync = useUpdateCaptureSettings();
  const syncRefusalId = useId();
  const queryClient = useQueryClient();
  // null = no unsaved change; the switch renders the stored posture.
  const [pending, setPending] = useState<boolean | null>(null);
  // The two settings on this card are saved apart. They point in OPPOSITE
  // directions — one withholds mail from colleagues, the other permits a seat
  // to hand it over on arrival — so a single Save button committing both at
  // once would let a reader flip one while meaning the other.
  const [pendingShared, setPendingShared] = useState<boolean | null>(null);
  const save = useMutation({
    mutationFn: async (mailSharing: boolean) => {
      return unwrap(
        await api.PATCH("/capture/settings", {
          body: { mail_sharing: mailSharing },
        }),
      );
    },
    onSuccess: (data) => {
      queryClient.setQueryData(["capture-settings"], data);
      setPending(null);
      toast.show(t("settings.saved"));
    },
  });

  const saveShared = useMutation({
    mutationFn: async (allowed: boolean) => {
      return unwrap(
        await api.PATCH("/capture/settings", {
          body: { shared_posture_allowed: allowed },
        }),
      );
    },
    onSuccess: (data) => {
      queryClient.setQueryData(["capture-settings"], data);
      setPendingShared(null);
      toast.show(t("settings.saved"));
    },
  });

  const notAdmin = canManage ? undefined : t("captureSettings.adminOnly");
  const syncRefused = sync.isError ? syncRefusalId : undefined;
  return (
    <Panel title={t("mailSharing.cardTitle")}>
      <QueryGate query={query} pendingLabel={t("mailSharing.cardTitle")}>
        {(settings) => {
          const shown = pending ?? settings.mail_sharing;
          const dirty = pending !== null && pending !== settings.mail_sharing;
          const sharedShown = pendingShared ?? settings.shared_posture_allowed;
          const sharedDirty =
            pendingShared !== null &&
            pendingShared !== settings.shared_posture_allowed;
          const sharedBand: BandState = {
            notice: sharedShown ? (
              <Callout
                kind="standing"
                tone="warning"
                title={t("mailSharing.sharedPosture.warningTitle")}
              >
                {t("mailSharing.sharedPosture.warning")}
              </Callout>
            ) : null,
            error: saveShared.isError ? saveShared.error : undefined,
            dirty: sharedDirty,
          };
          const sharingBand: BandState = {
            notice: shown ? null : (
              <Callout
                kind="standing"
                tone="warning"
                title={t("mailSharing.dangerTitle")}
              >
                {t("mailSharing.danger")}
              </Callout>
            ),
            error: save.isError ? save.error : undefined,
            dirty,
          };
          return (
            <>
              <SettingList bleed="settings">
                {/* The row draws the naming, so the switch carries the same
                    words as its hidden label rather than a second heading and
                    a hint of its own beside them. */}
                <SettingRow
                  label={t("mailSharing.label")}
                  description={t("mailSharing.help")}
                  // Without the row's description, a screen reader never
                  // hears what sharing does.
                  control={(control) => (
                    <Switch
                      describedBy={control["aria-describedby"]}
                      testId="mail-sharing-toggle"
                      label={t("mailSharing.label")}
                      labelHidden
                      reason={lockReasonOr(
                        settings.locks,
                        "capture.mail_sharing",
                        t,
                        notAdmin,
                      )}
                      checked={shown}
                      disabled={!canManage || save.isPending}
                      onChange={(next) => setPending(next)}
                    />
                  )}
                />
                {/* The one capture setting whose default withholds, so switching
                    it on is the permissive answer and the warning fires on true. */}
                <SettingRow
                  label={t("mailSharing.sharedPosture.label")}
                  description={t("mailSharing.sharedPosture.help")}
                  control={(control) => (
                    <Switch
                      describedBy={control["aria-describedby"]}
                      testId="shared-posture-allowed-toggle"
                      label={t("mailSharing.sharedPosture.label")}
                      labelHidden
                      reason={lockReasonOr(
                        settings.locks,
                        "capture.shared_posture_allowed",
                        t,
                        notAdmin,
                      )}
                      checked={sharedShown}
                      disabled={!canManage || saveShared.isPending}
                      onChange={(next) => setPendingShared(next)}
                    />
                  )}
                />
                <PacingRows
                  settings={settings}
                  locked={!canManage || sync.isPending}
                  notAdmin={notAdmin}
                  describedBy={syncRefused}
                  onPatch={(patch) => sync.mutate(patch)}
                />
              </SettingList>
              {(bandShows(sharedBand) || bandShows(sharingBand)) && (
                <PanelBody>
                  <CommitBand
                    {...sharedBand}
                    pending={saveShared.isPending}
                    onSave={() => {
                      if (pendingShared !== null) {
                        saveShared.mutate(pendingShared);
                      }
                    }}
                  />
                  <CommitBand
                    {...sharingBand}
                    pending={save.isPending}
                    onSave={() => {
                      if (pending !== null) {
                        save.mutate(pending);
                      }
                    }}
                  />
                </PanelBody>
              )}
            </>
          );
        }}
      </QueryGate>
      <PanelNotices
        refused={
          sync.isError
            ? {
                title: t("captureSettings.updateFailed"),
                error: sync.error,
                id: syncRefusalId,
              }
            : undefined
        }
      />
    </Panel>
  );
}

/**
 * What the sharing rule currently SAYS, on the reader's own connections page.
 *
 * A value, not a control. The card that changes these two settings lives on
 * Capture rules, because it writes the WORKSPACE's posture — everybody's mail,
 * not this reader's — and this page is the reader's own mailboxes. The rule
 * still governs the mail those mailboxes bring in, so a reader looking at their
 * connections and wondering who else can read it deserves the answer where the
 * question occurs to them.
 *
 * The link appears only for a reader who may CHANGE the rule — the same grant
 * and seat the switch itself asks for. Testing whether they may open the page
 * would be a wider question than the link makes: every seeded role may read the
 * capture posture, so a rep would follow "Change this" to a disabled switch.
 */
export function MailSharingPostureRow() {
  const t = useT();
  const query = useCaptureSettings();
  // The same question the switch on the other page asks, not "may they open
  // that page". Every seeded role may READ the capture posture, so a visibility
  // test offered "Change this" to a rep who would arrive at a disabled switch —
  // an invitation to a refusal, which is worse than no invitation.
  const canChangeIt = useCanWrite("capture_settings", "update");
  const shared = query.data?.mail_sharing ?? false;
  return (
    <Panel title={t("mailSharing.title")}>
      <SettingList bleed="settings">
        <SettingRow
          testId="mail-sharing-posture"
          label={t("mailSharing.label")}
          // Absent while the read is in flight: a wrong statement about who can
          // read somebody's mail is worse than a moment with none.
          description={
            query.isSuccess
              ? t(
                  shared
                    ? "mailSharing.posture.shared"
                    : "mailSharing.posture.private",
                )
              : undefined
          }
          control={
            canChangeIt && (
              <a href="#/settings/capture">{t("mailSharing.posture.where")}</a>
            )
          }
        />
      </SettingList>
    </Panel>
  );
}
