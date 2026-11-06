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
import { ErrorLine } from "../../design-system/errorline";
import { Panel, PanelBody, PanelRow } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { Select } from "../../design-system/select";
import { holdsEveryWord, searchWords } from "../../format/collate";
import {
  formatDate,
  formatNumber,
  identifierNumber,
} from "../../format/format";
import { useLocale, usePlural, useT } from "../../i18n";
import { throwProblem } from "../../lib/problem";
import { useViewerZone } from "../../lib/viewerzone";
import { isAccessKind, WhoCanSeePanel } from "./recordaccesspanel";
import { ShareConfirmation } from "./share.confirmation";
import {
  ApprovalRequiredError,
  RevokeGrantDialog,
  shareRefusalMessage,
} from "./sharerevoke";

export function ShareScreenBody({
  recordType,
  recordId,
  contactsOnly = false,
}: Readonly<{
  recordType: RecordType;
  recordId: string;
  contactsOnly?: boolean;
}>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  const zone = useViewerZone();
  const queryClient = useQueryClient();
  // The picker survives both dialogs when their original triggers disappear.
  const subjectField = useRef<HTMLInputElement>(null);
  const returnFocusToSubject = () => subjectField.current;
  const grantsKey = ["record-grants", recordType, recordId];

  const grantsQuery = useQuery({
    queryKey: grantsKey,
    enabled: !contactsOnly,
    queryFn: () => fetchGrants(recordType, recordId),
  });

  const { roster, usersQuery, teamsQuery, rosterPartial } = useShareRoster();

  const heldBySubject = useMemo(() => {
    const bySubject = new Map<string, RecordGrant>();
    for (const grantRow of contactsOnly ? [] : (grantsQuery.data ?? [])) {
      bySubject.set(
        subjectKey(grantRow.subject_type, grantRow.subject_id),
        grantRow,
      );
    }
    return bySubject;
  }, [grantsQuery.data, contactsOnly]);

  const [term, setTerm] = useState("");
  const [subject, setSubject] = useState<RosterSubject | null>(null);
  const [access, setAccess] = useState<Access>("read");
  const [expiryDays, setExpiryDays] = useState(7);
  const [reason, setReason] = useState("");
  const [sharedResult, setSharedResult] = useState<{
    shared: number;
    skipped: number;
  } | null>(null);

  const filteredRoster = useMemo(() => {
    const words = searchWords(term);
    if (words.length === 0) {
      return roster;
    }
    return roster.filter((r) => holdsEveryWord(words, [r.name, r.note]));
  }, [roster, term]);

  function resetForm() {
    setTerm("");
    setSubject(null);
    setAccess("read");
    setExpiryDays(7);
    setReason("");
  }

  const [unchanged, setUnchanged] = useState<{
    name: string;
    access: Access;
  } | null>(null);
  const [confirmationDraft, setConfirmationDraft] = useState<GrantDraft | null>(
    null,
  );
  const [askingConfirmation, setAskingConfirmation] = useState(false);

  const grant = useMutation({
    mutationFn: async (draft: GrantDraft) => {
      const body: Omit<CreateRecordGrantRequest, "record_type" | "record_id"> =
        {
          subject_type: draft.subject.kind,
          subject_id: draft.subject.id,
          access: draft.access,
          reason: draft.reason,
          expires_at: draft.expiresAt,
        };
      if (contactsOnly) {
        const { data, error } = await api.POST(
          "/companies/{id}/share-contacts",
          {
            params: { path: { id: recordId } },
            body,
          },
        );
        if (error) throwProblem(error);
        return data;
      }
      const { data, error } = await api.POST("/record-grants", {
        body: { ...body, record_type: recordType, record_id: recordId },
      });
      if (error) {
        if (error.code === "approval_required") {
          throw new ApprovalRequiredError();
        }
        throwProblem(error);
      }
      return data;
    },
    onSuccess: (data, draft) => {
      if (data && "shared" in data) {
        setSharedResult(data);
        queryClient.invalidateQueries({ queryKey: ["contact"] });
        queryClient.invalidateQueries({ queryKey: ["contact360"] });
        queryClient.invalidateQueries({
          queryKey: ["record-grants", "contact"],
        });
      }
      queryClient.invalidateQueries({ queryKey: grantsKey });
      setUnchanged(
        draft.change === "unchanged"
          ? { name: draft.subject.name, access: draft.access }
          : null,
      );
      setAskingConfirmation(false);
      resetForm();
    },
  });

  const [revokingId, setRevokingId] = useState<string | null>(null);

  const grantErrorMessage = grant.isError
    ? shareRefusalMessage(grant.error, t)
    : null;

  function dismissGrantFeedback() {
    if (grant.isError) {
      grant.reset();
    }
    setUnchanged(null);
    setSharedResult(null);
  }

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

  // Lowering an existing share needs confirmation before it removes access.
  function submit(picked: RosterSubject) {
    const draft = draftFor(picked);
    if (contactsOnly || draft.change === "lower") {
      setConfirmationDraft(draft);
      setAskingConfirmation(true);
      return;
    }
    grant.mutate(draft);
  }

  return (
    <div className="wrap share-screen">
      <Panel title={t(contactsOnly ? "share.companyContacts" : "share.title")}>
        <PanelBody>
          <div className="share-backlink">
            <Link2 aria-hidden />
            <EntityRef kind={recordType} id={recordId} />
          </div>
          {contactsOnly ? (
            <p>{t("share.companyContactsNote")}</p>
          ) : (
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
          )}
        </PanelBody>
      </Panel>

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
                  setAccess(heldGrant.access);
                  setReason(heldGrant.reason ?? "");
                  if (!heldGrant.expires_at) {
                    setExpiryDays(0);
                  }
                } else {
                  setAccess(candidate.kind === "team" ? "write" : "read");
                  setExpiryDays(candidate.kind === "team" ? 0 : 7);
                }
                dismissGrantFeedback();
              }}
            />
          </div>

          <div className="field">
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

          {sharedResult && (
            <p role="status">
              {t("share.companyContactsResult", {
                shared: formatNumber(sharedResult.shared, locale),
                skipped: formatNumber(sharedResult.skipped, locale),
              })}
            </p>
          )}
          {grantErrorMessage && <ErrorLine>{grantErrorMessage}</ErrorLine>}
        </PanelBody>
      </Panel>

      {!contactsOnly && (
        <>
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
                          <Badge
                            tone={g.access === "write" ? "accent" : undefined}
                          >
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
        </>
      )}

      <ShareConfirmation
        draft={confirmationDraft}
        bulk={contactsOnly}
        open={askingConfirmation}
        onClose={() => {
          setAskingConfirmation(false);
          grant.reset();
        }}
        returnFocusTo={returnFocusToSubject}
        onConfirm={(draft) => grant.mutate(draft)}
        pending={grant.isPending}
        error={grantErrorMessage}
      />
    </div>
  );
}
