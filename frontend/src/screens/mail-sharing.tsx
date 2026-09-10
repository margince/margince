import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { type ReactNode, useState } from "react";
import { api } from "../api/client";
import { useCanWrite } from "../app/capability";
import { Button } from "../design-system/atoms";
import { Callout } from "../design-system/callout";
import { Panel, PanelBody } from "../design-system/panel";
import { SettingList, SettingRow } from "../design-system/settingrow";
import { Switch } from "../design-system/switch";
import { useToast } from "../design-system/toast";
import { useT } from "../i18n";
import { problemMessageOf, QueryGate, throwProblem } from "./common";

// The workspace mail-sharing posture: ON by default, captured mail is
// readable by every colleague who can see the contact — the thing that makes
// the pipeline shared. Switching it OFF holds every email captured from then
// on to its participants, which makes shared CRM work hard, so the change is
// a deliberate act: a switch plus a Save button plus a warning that says the
// cost out loud, never a silent instant toggle. Every role sees the posture;
// only admin/ops may change it (same gating as the auto-enrich card).

function useMailSharing() {
  return useQuery({
    queryKey: ["capture-settings"],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/capture/settings");
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
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
}: Readonly<{
  notice: ReactNode;
  error: unknown;
  dirty: boolean;
  pending: boolean;
  onSave: () => void;
}>) {
  const t = useT();
  if (notice === null && error === undefined && !dirty) return null;
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
        <Button small variant="primary" disabled={pending} onClick={onSave}>
          {t("mailSharing.save")}
        </Button>
      )}
    </div>
  );
}

export function MailSharingCard() {
  const t = useT();
  const toast = useToast();
  const canManage = useCanWrite("capture_settings", "update");
  const query = useMailSharing();
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
      const { data, error } = await api.PATCH("/capture/settings", {
        body: { mail_sharing: mailSharing },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (data) => {
      queryClient.setQueryData(["capture-settings"], data);
      setPending(null);
      toast.show(t("settings.saved"));
    },
  });

  const saveShared = useMutation({
    mutationFn: async (allowed: boolean) => {
      const { data, error } = await api.PATCH("/capture/settings", {
        body: { shared_posture_allowed: allowed },
      });
      if (error) {
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (data) => {
      queryClient.setQueryData(["capture-settings"], data);
      setPendingShared(null);
      toast.show(t("settings.saved"));
    },
  });

  return (
    <Panel title={t("mailSharing.title")}>
      <PanelBody>
        <p className="settings-panel-sub">{t("mailSharing.sub")}</p>
        <QueryGate query={query} pendingLabel={t("mailSharing.title")}>
          {(settings) => {
            const shown = pending ?? settings.mail_sharing;
            const dirty = pending !== null && pending !== settings.mail_sharing;
            const sharedShown =
              pendingShared ?? settings.shared_posture_allowed;
            const sharedDirty =
              pendingShared !== null &&
              pendingShared !== settings.shared_posture_allowed;
            return (
              <>
                <SettingList>
                  {/* The row draws the naming, so the switch carries the same
                      words as its hidden label rather than a second heading and
                      a hint of its own beside them. */}
                  <SettingRow
                    label={t("mailSharing.label")}
                    description={t("mailSharing.help")}
                    // The function form, so the row's description reaches the
                    // switch: the sentence saying what sharing DOES used to be
                    // the switch's own `hint`, and moving it into the row would
                    // otherwise take it away from every reader who cannot see
                    // it. `labelHidden` still keeps the naming the row's.
                    control={(control) => (
                      <Switch
                        describedBy={control["aria-describedby"]}
                        testId="mail-sharing-toggle"
                        label={t("mailSharing.label")}
                        labelHidden
                        reason={
                          canManage ? undefined : t("captureSettings.adminOnly")
                        }
                        checked={shown}
                        disabled={!canManage || save.isPending}
                        onChange={(next) => setPending(next)}
                      />
                    )}
                  />
                  {/* Whether a seat may ask for `shared` at all. The only
                      capture setting whose default WITHHOLDS, so it is the one
                      row on this card where ON is the permissive answer — the
                      warning below fires on true rather than on false. */}
                  <SettingRow
                    label={t("mailSharing.sharedPosture.label")}
                    description={t("mailSharing.sharedPosture.help")}
                    control={(control) => (
                      <Switch
                        describedBy={control["aria-describedby"]}
                        testId="shared-posture-allowed-toggle"
                        label={t("mailSharing.sharedPosture.label")}
                        labelHidden
                        reason={
                          canManage ? undefined : t("captureSettings.adminOnly")
                        }
                        checked={sharedShown}
                        disabled={!canManage || saveShared.isPending}
                        onChange={(next) => setPendingShared(next)}
                      />
                    )}
                  />
                </SettingList>
                <CommitBand
                  notice={
                    sharedShown ? (
                      <Callout
                        kind="standing"
                        tone="warn"
                        title={t("mailSharing.sharedPosture.warningTitle")}
                      >
                        {t("mailSharing.sharedPosture.warning")}
                      </Callout>
                    ) : null
                  }
                  error={saveShared.isError ? saveShared.error : undefined}
                  dirty={sharedDirty}
                  pending={saveShared.isPending}
                  onSave={() => {
                    if (pendingShared !== null) {
                      saveShared.mutate(pendingShared);
                    }
                  }}
                />
                <CommitBand
                  notice={
                    shown ? null : (
                      <Callout
                        kind="standing"
                        tone="warn"
                        title={t("mailSharing.dangerTitle")}
                      >
                        {t("mailSharing.danger")}
                      </Callout>
                    )
                  }
                  error={save.isError ? save.error : undefined}
                  dirty={dirty}
                  pending={save.isPending}
                  onSave={() => {
                    if (pending !== null) {
                      save.mutate(pending);
                    }
                  }}
                />
              </>
            );
          }}
        </QueryGate>
      </PanelBody>
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
  const query = useMailSharing();
  // The same question the switch on the other page asks, not "may they open
  // that page". Every seeded role may READ the capture posture, so a visibility
  // test offered "Change this" to a rep who would arrive at a disabled switch —
  // an invitation to a refusal, which is worse than no invitation.
  const canChangeIt = useCanWrite("capture_settings", "update");
  const shared = query.data?.mail_sharing ?? false;
  return (
    <Panel title={t("mailSharing.title")}>
      <PanelBody>
        {/* A fact, not a setting: a definition list rather than SettingRow,
            which requires a control because every row of it is something a
            reader can operate. This one is operated on Capture rules. */}
        <dl className="settings-facts">
          <dt>{t("mailSharing.label")}</dt>
          <dd>
            {/* The stored answer in words. `query.data` is absent while the read
                is in flight, and an absent answer says nothing rather than
                claiming the narrower one — a wrong statement about who can read
                somebody's mail is worse than a moment with no statement. */}
            {query.isSuccess
              ? t(
                  shared
                    ? "mailSharing.posture.shared"
                    : "mailSharing.posture.private",
                )
              : null}
          </dd>
        </dl>
        {canChangeIt && (
          <p className="t-caption">
            <a href="#/settings/capture">{t("mailSharing.posture.where")}</a>
          </p>
        )}
      </PanelBody>
    </Panel>
  );
}
