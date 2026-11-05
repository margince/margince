import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Building2 } from "lucide-react";
import { useId, useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { AppLink } from "../design-system/applink";
import { Badge } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { NumberSettingRow } from "../design-system/numbersetting";
import { Panel, PanelBody, PanelPlate } from "../design-system/panel";
import { PanelNotices } from "../design-system/panelnotices";
import { QueryGate } from "../design-system/querystates";
import { useSavePage } from "../design-system/savepage";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { Switch } from "../design-system/switch";
import { useToast } from "../design-system/toast";
import { lockReasonOr } from "../format/settinglocks";
import { useT } from "../i18n";
import { useCanWrite } from "../lib/capability";
import { unwrap } from "../lib/problem";
import { useMe } from "../lib/session";
import {
  MAIL_REVIEW_BOUNDS,
  MAIL_SYNC_BOUNDS,
  useCaptureSettings,
  useUpdateCaptureSettings,
} from "./capture-settings";

// The workspace mail-sharing posture: ON by default, so captured mail is shared
// with every colleague who can see the contact. Every role reads it; only
// admin/ops may change it, and each switch writes on flip and toasts.

type CaptureSettingsPatch =
  components["schemas"]["UpdateCaptureSettingsRequest"];

/**
 * One switch's write. Each PATCH carries only its own field: the two settings
 * point in opposite directions, so one write must never also rewrite the other.
 */
function usePostureWrite(bodyOf: (next: boolean) => CaptureSettingsPatch) {
  const t = useT();
  const toast = useToast();
  const queryClient = useQueryClient();
  const [optimistic, setOptimistic] = useState<boolean | null>(null);
  const write = useMutation({
    mutationFn: async (next: boolean) =>
      unwrap(await api.PATCH("/capture/settings", { body: bodyOf(next) })),
    onSuccess: (data) => {
      queryClient.setQueryData(["capture-settings"], data);
      toast.show(t("settings.saved"));
    },
    onSettled: () => setOptimistic(null),
  });
  const flip = (next: boolean) => {
    setOptimistic(next);
    write.mutate(next);
  };
  return { write, optimistic, flip };
}

function PostureSwitch({
  testId,
  label,
  description,
  checked,
  canManage,
  reason,
  pending,
  onChange,
}: Readonly<{
  testId: string;
  label: string;
  description: string;
  checked: boolean;
  canManage: boolean;
  /** Why the switch is held: a lock, or the reader's role. */
  reason: string | undefined;
  pending: boolean;
  onChange: (next: boolean) => void;
}>) {
  return (
    <SettingRow
      label={label}
      description={description}
      // Without the row's description, a screen reader never hears what the
      // switch does.
      control={(control) => (
        <Switch
          describedBy={control["aria-describedby"]}
          testId={testId}
          label={label}
          labelHidden
          reason={reason}
          checked={checked}
          disabled={!canManage}
          pending={pending}
          onChange={onChange}
        />
      )}
    />
  );
}

export function MailSharingCard() {
  const t = useT();
  const canManage = useCanWrite("capture_settings", "update");
  const query = useCaptureSettings();
  const sync = useUpdateCaptureSettings();
  const syncRefusalId = useId();
  // In a page the bar saves the number and shows the refusal once more; the
  // field still names its own.
  const inPage = useSavePage() !== null;
  const sharing = usePostureWrite((next) => ({ mail_sharing: next }));
  const shared = usePostureWrite((next) => ({ shared_posture_allowed: next }));
  // One card, one refusal: the switch flipped last is the one the reader means.
  const refusedSwitch = [sharing.write, shared.write]
    .filter((write) => write.isError)
    .sort((a, b) => b.submittedAt - a.submittedAt)[0];

  const notAdmin = canManage ? undefined : t("captureSettings.adminOnly");
  const syncRefused = sync.isError ? syncRefusalId : undefined;
  return (
    <Panel title={t("mailSharing.cardTitle")}>
      <QueryGate query={query} pendingLabel={t("mailSharing.cardTitle")}>
        {(settings) => {
          const sharingOn = sharing.optimistic ?? settings.mail_sharing;
          const sharedOn = shared.optimistic ?? settings.shared_posture_allowed;
          return (
            <>
              <SettingList bleed="settings">
                <PostureSwitch
                  testId="mail-sharing-toggle"
                  label={t("mailSharing.label")}
                  description={t("mailSharing.help")}
                  reason={lockReasonOr(
                    settings.locks,
                    "capture.mail_sharing",
                    t,
                    notAdmin,
                  )}
                  checked={sharingOn}
                  canManage={canManage}
                  pending={sharing.write.isPending}
                  onChange={sharing.flip}
                />
                {/* The one capture setting whose default withholds, so on is the
                    permissive answer and its warning fires on true. */}
                <PostureSwitch
                  testId="shared-posture-allowed-toggle"
                  label={t("mailSharing.sharedPosture.label")}
                  description={t("mailSharing.sharedPosture.help")}
                  reason={lockReasonOr(
                    settings.locks,
                    "capture.shared_posture_allowed",
                    t,
                    notAdmin,
                  )}
                  checked={sharedOn}
                  canManage={canManage}
                  pending={shared.write.isPending}
                  onChange={shared.flip}
                />
                {/* Commits on its own, like every number row: pacing the
                    mailboxes withholds nothing from anybody. */}
                <NumberSettingRow
                  label={t("captureMailSync.interval.label")}
                  description={t("captureMailSync.interval.help")}
                  testId="capture-mail-sync"
                  value={settings.mail_sync_interval_seconds}
                  {...MAIL_SYNC_BOUNDS.mail_sync_interval_seconds}
                  refusal={t("captureMailSync.interval.refusal")}
                  disabled={!canManage || sync.isPending}
                  reason={lockReasonOr(
                    settings.locks,
                    "capture.mail_sync_interval_seconds",
                    t,
                    notAdmin,
                  )}
                  describedBy={syncRefused}
                  onCommit={(next) =>
                    inPage
                      ? sync.mutateAsync({ mail_sync_interval_seconds: next })
                      : sync.mutate({ mail_sync_interval_seconds: next })
                  }
                />
                <NumberSettingRow
                  label={t("captureReview.passCap.label")}
                  description={t("captureReview.passCap.help")}
                  testId="capture-review-pass-cap"
                  value={settings.confidentiality_pass_cap}
                  {...MAIL_REVIEW_BOUNDS.confidentiality_pass_cap}
                  refusal={t("captureReview.passCap.refusal")}
                  disabled={!canManage || sync.isPending}
                  reason={lockReasonOr(
                    settings.locks,
                    "capture.confidentiality_pass_cap",
                    t,
                    notAdmin,
                  )}
                  describedBy={syncRefused}
                  onCommit={(next) =>
                    inPage
                      ? sync.mutateAsync({ confidentiality_pass_cap: next })
                      : sync.mutate({ confidentiality_pass_cap: next })
                  }
                />
              </SettingList>
              <PostureCosts sharingOn={sharingOn} sharedOn={sharedOn} />
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
            : refusedSwitch === undefined
              ? undefined
              : {
                  title: t("mailSharing.saveFailed"),
                  error: refusedSwitch.error,
                }
        }
      />
    </Panel>
  );
}

/** What the current posture costs, said for as long as it stands. */
function PostureCosts({
  sharingOn,
  sharedOn,
}: Readonly<{ sharingOn: boolean; sharedOn: boolean }>) {
  const t = useT();
  if (sharingOn && !sharedOn) {
    return null;
  }
  return (
    <PanelBody className="panel-notices">
      {sharedOn && (
        <Callout
          kind="standing"
          tone="warning"
          title={t("mailSharing.sharedPosture.warningTitle")}
        >
          {t("mailSharing.sharedPosture.warning")}
        </Callout>
      )}
      {!sharingOn && (
        <Callout
          kind="standing"
          tone="warning"
          title={t("mailSharing.dangerTitle")}
        >
          {t("mailSharing.danger")}
        </Callout>
      )}
    </PanelBody>
  );
}

/**
 * What the sharing rule currently says, as the plate under the reader's own
 * Mail and calendar card. A value, not a control.
 *
 * The switch lives on Capture rules because it writes the workspace's posture,
 * and the badge says so. The plate sits under the mailboxes because it governs
 * the mail they bring in.
 */
export function MailSharingPlate() {
  const t = useT();
  const query = useCaptureSettings();
  // The same question the switch on the other page asks, not "may they open
  // that page". Every seeded role may read the capture posture, so a
  // visibility test offered "Change this" to a rep who would find a disabled
  // switch.
  const canChangeIt = useCanWrite("capture_settings", "update");
  const company = useMe().data?.workspace_name ?? "";
  // Nothing until the read lands: a wrong statement about who can read
  // somebody's mail is worse than a moment with no statement.
  if (!query.isSuccess) {
    return null;
  }
  return (
    <PanelPlate>
      <Badge icon={Building2}>
        <span className="sr-only">
          {t("settings.scopeAria", { scope: t("mailSharing.plate.scope") })}
        </span>
        <span aria-hidden="true">{t("mailSharing.plate.scope")}</span>
      </Badge>
      <p>
        {t(
          query.data.mail_sharing
            ? "mailSharing.plate.shared"
            : "mailSharing.plate.private",
          { company },
        )}{" "}
        {canChangeIt && (
          <AppLink href="#/settings/capture">
            {t("mailSharing.posture.where")}
          </AppLink>
        )}
      </p>
    </PanelPlate>
  );
}
