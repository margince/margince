import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link2, ShieldCheck } from "lucide-react";
import { useMemo, useRef, useState } from "react";
import { api } from "../../api/client";
import { EntityRef } from "../../components/entityref";
import {
  type Access,
  accessLabel,
  type CreateRecordGrantRequest,
  type DraftFields,
  EXPIRY_OPTIONS,
  expiresAtFor,
  fetchGrants,
  type GrantDraft,
  type ReassertDraft,
  type RecordGrant,
  type RecordType,
  RosterPicker,
  type RosterSubject,
  reassertKind,
  SubjectKindIcon,
  subjectKey,
  useShareRoster,
} from "../../components/share.parts";
import {
  Badge,
  Button,
  Field,
  SearchField,
  SegmentedControl,
  Textarea,
} from "../../design-system/atoms";
import { ConfirmModal } from "../../design-system/confirmmodal";
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import {
  formatDate,
  formatNumber,
  identifierNumber,
} from "../../format/format";
import { viewerZone } from "../../format/timezone";
import { useLocale, usePlural, useT } from "../../i18n";
import { throwProblem } from "../../lib/problem";
import { isAccessKind, WhoCanSeePanel } from "./recordaccesspanel";
import {
  ApprovalRequiredError,
  RevokeGrantDialog,
  shareRefusalMessage,
} from "./sharerevoke";

export function ShareScreenBody({
  recordType,
  recordId,
}: Readonly<{ recordType: RecordType; recordId: string }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  // Grant expiry must read in the viewer's own timezone. The browser's
  // resolved IANA zone answers "what calendar date does this viewer see".
  const zone = viewerZone();
  const queryClient = useQueryClient();
  // Where focus lands when a dialog closes on a control that is gone. Both
  // dialogs destroy their own trigger on success: a revoked grant's row leaves
  // the roster, and a downgrade clears the picker that opened it. Otherwise
  // focus falls to the document body and a keyboard reader starts over. The
  // subject field is the one control always present, so focus lands there.
  const subjectField = useRef<HTMLInputElement>(null);
  const returnFocusToSubject = () => subjectField.current;
  const grantsKey = ["record-grants", recordType, recordId];

  const grantsQuery = useQuery({
    queryKey: grantsKey,
    queryFn: () => fetchGrants(recordType, recordId),
  });

  const { roster, usersQuery, teamsQuery, rosterPartial } = useShareRoster();

  // At most one grant per subject on one record, since the create is idempotent
  // on `(record, subject)`. So the grant itself is the value: the picker row
  // states the level, and a re-assert is measured against it.
  const heldBySubject = useMemo(() => {
    const bySubject = new Map<string, RecordGrant>();
    for (const grantRow of grantsQuery.data ?? []) {
      bySubject.set(
        subjectKey(grantRow.subject_type, grantRow.subject_id),
        grantRow,
      );
    }
    return bySubject;
  }, [grantsQuery.data]);

  const [term, setTerm] = useState("");
  const [subject, setSubject] = useState<RosterSubject | null>(null);
  const [access, setAccess] = useState<Access>("read");
  const [expiryDays, setExpiryDays] = useState(7);
  const [reason, setReason] = useState("");

  const filteredRoster = useMemo(() => {
    const q = term.trim().toLowerCase();
    if (!q) {
      return roster;
    }
    return roster.filter((r) =>
      `${r.name} ${r.note}`.toLowerCase().includes(q),
    );
  }, [roster, term]);

  function resetForm() {
    setTerm("");
    setSubject(null);
    setAccess("read");
    setExpiryDays(7);
    setReason("");
  }

  // A restatement that moved nothing, kept so the screen can say so. The
  // subject's name and level are copied out of the draft rather than read back
  // off the form, which the success has already cleared.
  const [unchanged, setUnchanged] = useState<{
    name: string;
    access: Access;
  } | null>(null);
  // The draft a downgrade is waiting on. Confirming submits this draft rather
  // than re-reading the form behind the dialog. Closing keeps it: the copy
  // names the subject and both levels, so a dropped draft asks about nobody.
  const [downgrade, setDowngrade] = useState<ReassertDraft | null>(null);
  const [askingDowngrade, setAskingDowngrade] = useState(false);

  // The whole submit arrives as the mutation's variable, not through this
  // closure. React-query re-arms a mutation's options in a passive effect. A
  // submit between the commit that enables the button and that effect would
  // run the previous render's function, where nothing is picked yet.
  const grant = useMutation({
    mutationFn: async (draft: GrantDraft) => {
      const body: CreateRecordGrantRequest = {
        record_type: recordType,
        record_id: recordId,
        subject_type: draft.subject.kind,
        subject_id: draft.subject.id,
        access: draft.access,
        reason: draft.reason,
        expires_at: draft.expiresAt,
      };
      const { data, error } = await api.POST("/record-grants", { body });
      if (error) {
        if (error.code === "approval_required") {
          throw new ApprovalRequiredError();
        }
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (_data, draft) => {
      queryClient.invalidateQueries({ queryKey: grantsKey });
      // A re-assert of the same level, expiry and reason leaves the list below
      // looking as it did. Silence there reads as "my change landed",
      // which is the one thing that did not happen, so this press says so.
      setUnchanged(
        draft.change === "unchanged"
          ? { name: draft.subject.name, access: draft.access }
          : null,
      );
      setAskingDowngrade(false);
      resetForm();
    },
  });

  const [revokingId, setRevokingId] = useState<string | null>(null);

  const grantErrorMessage = grant.isError
    ? shareRefusalMessage(grant.error, t)
    : null;

  // A stale grant error, or a "nothing changed" from the last press, must not
  // outlive the edit that could change the answer. Clearing both on any field
  // change mirrors the revoke path's reset(). The react-query reset
  // is guarded so a keystroke in an already-clean form doesn't churn its state.
  function dismissGrantFeedback() {
    if (grant.isError) {
      grant.reset();
    }
    setUnchanged(null);
  }

  // Everything this press means, decided here in the committed render and
  // handed to the mutation whole.
  function draftFor(picked: RosterSubject): GrantDraft {
    const fields: DraftFields = {
      subject: picked,
      access,
      expiresAt: expiresAtFor(expiryDays),
      reason: reason.trim() || undefined,
    };
    const heldGrant = heldBySubject.get(subjectKey(picked.kind, picked.id));
    if (!heldGrant) {
      return { ...fields, change: "new" };
    }
    return {
      ...fields,
      change: reassertKind(heldGrant, fields),
      heldAccess: heldGrant.access,
    };
  }

  // Lowering a colleague's level is the one direction the actor may not have
  // meant, so it is asked about first. Raising, restating, or a first grant go
  // straight through: none removes an authority somebody already relies on.
  function submit(picked: RosterSubject) {
    const draft = draftFor(picked);
    if (draft.change === "lower") {
      setDowngrade(draft);
      setAskingDowngrade(true);
      return;
    }
    grant.mutate(draft);
  }

  return (
    <div className="wrap share-screen">
      <Panel title={t("share.title")}>
        <PanelBody>
          <div className="share-backlink">
            <Link2 aria-hidden />
            <EntityRef kind={recordType} id={recordId} />
          </div>
          <p className="share-ceiling t-caption">
            <ShieldCheck aria-hidden />
            <span>
              {t("share.ceiling.pre")}
              <b>{t("share.ceiling.recordEmphasis")}</b>
              {t("share.ceiling.mid")}
              <b>{t("share.ceiling.noWider")}</b>
              {t("share.ceiling.post")}
            </span>
          </p>
        </PanelBody>
      </Panel>

      {/* The ceiling is server-enforced: a POST that exceeds the granter's own
          access comes back 422 / approval_required and is surfaced below. No
          endpoint returns this reader's access for this record, so the surface
          states the rule rather than drawing a limit it cannot know. */}
      <Panel
        title={t("share.grantAccess")}
        actions={
          <Button
            variant="primary"
            disabled={!subject}
            pending={grant.isPending}
            onClick={() => subject && submit(subject)}
            data-testid="share-grant-submit"
          >
            {/* A subject who already holds a grant is not being granted one:
                the press restates what they hold, and the word on the button
                is the reader's last cue to which of the two they are doing. */}
            {subject && heldBySubject.has(subjectKey(subject.kind, subject.id))
              ? t("share.update")
              : t("share.grant")}
          </Button>
        }
      >
        <PanelBody className="form-stack">
          <div className="field">
            <Field label={t("share.subject")}>
              {(control) => (
                <SearchField
                  {...control}
                  ref={subjectField}
                  placeholder={t("share.subject")}
                  value={term}
                  onChange={(event) => {
                    setTerm(event.target.value);
                    setSubject(null);
                    dismissGrantFeedback();
                  }}
                />
              )}
            </Field>
            <RosterPicker
              usersQuery={usersQuery}
              teamsQuery={teamsQuery}
              partial={rosterPartial}
              filteredRoster={filteredRoster}
              held={heldBySubject}
              subject={subject}
              t={t}
              onPick={(candidate) => {
                setSubject(candidate);
                setTerm(candidate.name);
                const heldGrant = heldBySubject.get(
                  subjectKey(candidate.kind, candidate.id),
                );
                if (heldGrant) {
                  // The form opens on what this subject holds today, because a
                  // re-assert restates every field. On the compose defaults,
                  // Update would downgrade a write holder to read and clear the
                  // reason. A set expiry has no day count to return to (the four
                  // options are relative to now). So that control stays put, and
                  // the consequence line below says what the chosen option does.
                  setAccess(heldGrant.access);
                  setReason(heldGrant.reason ?? "");
                  if (!heldGrant.expires_at) {
                    setExpiryDays(0);
                  }
                }
                dismissGrantFeedback();
              }}
            />
          </div>

          <div className="field">
            {/* A span, not a label: a segmented control is a group of buttons,
                with no single labelable element for a `for` to point at. A
                label aimed at the wrapper resolves to nothing and exposes no
                name. The group carries its own accessible name instead. */}
            <span className="t-label">{t("share.access")}</span>
            <div>
              <SegmentedControl
                options={["read", "write"] as const}
                value={access}
                label={t("share.access")}
                onChange={(next) => {
                  setAccess(next);
                  dismissGrantFeedback();
                }}
                labels={{
                  read: t("share.access.read"),
                  write: t("share.access.write"),
                }}
              />
            </div>
            <p className="t-caption">
              {access === "read"
                ? t("share.access.readNote")
                : t("share.access.writeNote")}
            </p>
          </div>

          <Field label={t("share.expiry")}>
            {(control) => (
              <Select
                {...control}
                // A day count is the state; the control speaks strings, so the
                // conversion happens here at the boundary and nowhere else.
                value={identifierNumber(expiryDays)}
                onChange={(value) => {
                  setExpiryDays(Number(value));
                  dismissGrantFeedback();
                }}
                options={EXPIRY_OPTIONS.map((option) => ({
                  value: identifierNumber(option.days),
                  label: t(option.key),
                }))}
              />
            )}
          </Field>

          {/* A share widens who can see a record, so the surface states the
              consequence in plain words rather than leaving the reader to
              infer it from a duration. Access ending on its own is the
              default, and the one case that does not end says so. */}
          <p className="t-caption" data-testid="share-expiry-consequence">
            {expiryDays > 0
              ? plural("share.expiryConsequence", expiryDays, {
                  days: formatNumber(expiryDays, locale),
                })
              : t("share.expiryConsequenceNone")}
          </p>

          <Field label={t("share.reason")}>
            {(control) => (
              <Textarea
                {...control}
                className="share-reason"
                value={reason}
                onChange={(event) => {
                  setReason(event.target.value);
                  dismissGrantFeedback();
                }}
              />
            )}
          </Field>

          {unchanged && (
            <p
              className="t-caption"
              role="status"
              data-testid="share-unchanged"
            >
              {t("share.unchanged", {
                name: unchanged.name,
                access: accessLabel(unchanged.access, t),
              })}
            </p>
          )}

          {grantErrorMessage && <ErrorLine>{grantErrorMessage}</ErrorLine>}
        </PanelBody>
      </Panel>

      {isAccessKind(recordType) && (
        <WhoCanSeePanel kind={recordType} recordId={recordId} />
      )}
      <Panel title={t("share.whoHasAccess")}>
        <QueryGate
          query={grantsQuery}
          empty={(rows) => rows.length === 0}
          pendingLabel={t("share.whoHasAccess")}
        >
          {(rows) => (
            <div data-testid="share-acl-list">
              {rows.map((g) => (
                <PanelRow key={g.id} className="share-acl-row">
                  <div className="share-acl-who">
                    <span className="share-acl-name">
                      <SubjectKindIcon kind={g.subject_type} t={t} />
                      <EntityRef kind={g.subject_type} id={g.subject_id} />
                    </span>
                    <div className="share-acl-meta">
                      {/* The same badge, in the same tone, as the picker row
                          for this subject: one drawing of a level per screen,
                          or the reader has two things to learn instead of one. */}
                      <Badge tone={g.access === "write" ? "accent" : undefined}>
                        {accessLabel(g.access, t)}
                      </Badge>
                      <span className="t-caption">
                        {t("share.grantedBy")}{" "}
                        <EntityRef kind="user" id={g.granted_by} />
                      </span>
                      {g.reason && (
                        <span className="t-caption">{g.reason}</span>
                      )}
                      {g.expires_at && (
                        <Badge tone="warning">
                          {formatDate(g.expires_at, locale, zone)}
                        </Badge>
                      )}
                    </div>
                  </div>
                  <Button
                    variant="danger"
                    onClick={() => setRevokingId(g.id)}
                    data-testid="revoke-grant"
                  >
                    {t("share.revoke")}
                  </Button>
                </PanelRow>
              ))}
            </div>
          )}
        </QueryGate>
      </Panel>

      <RevokeGrantDialog
        grant={grantsQuery.data?.find((g) => g.id === revokingId) ?? null}
        grantsKey={grantsKey}
        onClose={() => setRevokingId(null)}
        returnFocusTo={returnFocusToSubject}
      />

      {downgrade !== null && (
        <ConfirmModal
          open={askingDowngrade}
          onClose={() => {
            setAskingDowngrade(false);
            grant.reset();
          }}
          title={t("share.downgradeTitle")}
          confirmLabel={t("share.downgradeConfirm", {
            to: accessLabel(downgrade.access, t),
          })}
          confirmVariant="danger"
          returnFocusTo={returnFocusToSubject}
          onConfirm={() => grant.mutate(downgrade)}
          pending={grant.isPending}
          error={grantErrorMessage}
        >
          <p data-testid="share-downgrade-body">
            {t("share.downgradeBody", {
              name: downgrade.subject.name,
              from: accessLabel(downgrade.heldAccess, t),
              to: accessLabel(downgrade.access, t),
            })}
          </p>
        </ConfirmModal>
      )}
    </div>
  );
}
