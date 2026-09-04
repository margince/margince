import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../api/client";
import type { components } from "../api/schema";
import { Badge, Button, DataTable, EmptyState } from "../design-system/atoms";
import { ConfirmModal } from "../design-system/confirmmodal";
import { Panel, PanelBody } from "../design-system/panel";
import { useToast } from "../design-system/toast";
import { formatDate } from "../format/format";
import { viewerZone } from "../format/timezone";
import { useLocale, useT } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { problemMessageOf, QueryGate, throwProblem } from "./common";

// What the classifier decided about each sender this mailbox brought in, and
// the seat's own answer where they gave one.
//
// The page exists because the posture rests on being checkable. A product that
// decides silently which of your correspondents becomes a contact — and which
// of them is family, and whose mail gets destroyed — is one a person has to
// trust rather than audit. This is the audit.
//
// Read-only for everybody but the owner: the endpoint answers the caller's own
// senders and has no admin view, because whose mail a person keeps out is
// itself private.

type SenderDecision = components["schemas"]["CaptureSenderDecision"];

// The classifier's vocabulary, in the reader's words. A kind absent from the
// map is one the server learned to say and this screen has not — it falls back
// to the raw token rather than rendering nothing, so a new kind shows up as
// something to name instead of a blank cell.
const kindLabel: Record<string, MessageKey> = {
  person: "senders.kind.person",
  role_mailbox: "senders.kind.roleMailbox",
  organization_sender: "senders.kind.organizationSender",
  newsletter: "senders.kind.newsletter",
  transactional: "senders.kind.transactional",
  spam: "senders.kind.spam",
  personal: "senders.kind.personal",
  advisor: "senders.kind.advisor",
};

// Which kinds mean "this sender's mail is in the CRM as a contact". The tone
// carries it at a glance down the column; the words still say it, because
// colour is never the only signal.
const admitted = new Set(["person", "role_mailbox", "organization_sender"]);

function useSenders() {
  return useQuery({
    queryKey: ["capture-senders"],
    queryFn: async () => {
      const { data, error, response } = await api.GET("/capture/senders");
      if (error || !response.ok) {
        throwProblem(error);
      }
      return data;
    },
  });
}

function useSetDecision() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const t = useT();
  return useMutation({
    // Address and decision both arrive as variables: the row a reader pressed
    // belongs to the committed render, and a mutationFn reading render state
    // would answer with the previous one (frontend/AGENTS.md,
    // mutation-variable-coverage).
    mutationFn: async (vars: {
      address: string;
      decision: "business" | "keep_out";
    }) => {
      const { error } = await api.PUT("/capture/senders/{address}/decision", {
        params: { path: { address: vars.address } },
        body: { decision: vars.decision },
      });
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["capture-senders"] });
      toast.show(t("settings.saved"));
    },
  });
}

function useWithdrawDecision() {
  const queryClient = useQueryClient();
  const toast = useToast();
  const t = useT();
  return useMutation({
    mutationFn: async (address: string) => {
      const { error } = await api.DELETE(
        "/capture/senders/{address}/decision",
        { params: { path: { address } } },
      );
      if (error) {
        throwProblem(error);
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["capture-senders"] });
      toast.show(t("settings.saved"));
    },
  });
}

export function CaptureSendersCard() {
  const t = useT();
  const query = useSenders();
  const setDecision = useSetDecision();
  const withdraw = useWithdrawDecision();
  // Keeping a sender out destroys the mail they already brought in, so it asks
  // first. Readmitting one does not, because nothing is lost by it.
  const [keepingOut, setKeepingOut] = useState<string | null>(null);

  return (
    <Panel title={t("senders.title")}>
      <PanelBody>
        <p className="settings-panel-sub">{t("senders.sub")}</p>
        <QueryGate query={query} pendingLabel={t("senders.title")}>
          {(list) =>
            list.data.length === 0 ? (
              <EmptyState title={t("senders.emptyTitle")}>
                {t("senders.emptyBody")}
              </EmptyState>
            ) : (
              <>
                <DataTable<SenderDecision>
                  label={t("senders.title")}
                  rows={list.data}
                  rowKey={(row) => row.address}
                  columns={[
                    {
                      key: "address",
                      header: t("senders.colSender"),
                      render: (row) => row.address,
                    },
                    {
                      key: "decision",
                      header: t("senders.colDecision"),
                      render: (row) => <DecisionCell row={row} />,
                    },
                    {
                      key: "record",
                      header: t("senders.colRecord"),
                      render: (row) =>
                        row.record_exists
                          ? t("senders.recordYes")
                          : t("senders.recordNo"),
                    },
                    {
                      key: "verbs",
                      header: t("senders.colActions"),
                      render: (row) => (
                        <div className="cell-actions">
                          {/* The two verbs are the two answers the contract
                              takes. A sender already admitted needs no
                              readmitting, so only the other one is offered. */}
                          {!admitted.has(row.kind ?? "") &&
                            row.decision !== "business" && (
                              <Button
                                small
                                variant="ghost"
                                disabled={setDecision.isPending}
                                onClick={() =>
                                  setDecision.mutate({
                                    address: row.address,
                                    decision: "business",
                                  })
                                }
                              >
                                {t("senders.markBusiness")}
                              </Button>
                            )}
                          {row.decision !== "keep_out" && (
                            <Button
                              small
                              variant="ghost"
                              disabled={setDecision.isPending}
                              onClick={() => setKeepingOut(row.address)}
                            >
                              {t("senders.keepOut")}
                            </Button>
                          )}
                          {row.overruled && (
                            <Button
                              small
                              variant="ghost"
                              disabled={withdraw.isPending}
                              onClick={() => withdraw.mutate(row.address)}
                            >
                              {t("senders.withdraw")}
                            </Button>
                          )}
                        </div>
                      ),
                    },
                  ]}
                />
                {setDecision.isError && (
                  <p className="settings-panel-sub" role="alert">
                    {problemMessageOf(setDecision.error, t)}
                  </p>
                )}
              </>
            )
          }
        </QueryGate>
        <ConfirmModal
          open={keepingOut !== null}
          onClose={() => setKeepingOut(null)}
          title={t("senders.keepOutTitle")}
          confirmLabel={t("senders.keepOutConfirm")}
          confirmVariant="danger"
          pending={setDecision.isPending}
          onConfirm={() => {
            if (keepingOut) {
              setDecision.mutate(
                { address: keepingOut, decision: "keep_out" },
                { onSuccess: () => setKeepingOut(null) },
              );
            }
          }}
        >
          {t("senders.keepOutBody")}
        </ConfirmModal>
      </PanelBody>
    </Panel>
  );
}

// What was decided, and by whom. A seat's own answer outranks the classifier's
// and says so: "you decided" beside the kind it replaced, so the row reports a
// correction rather than only the answer that now stands.
function DecisionCell({ row }: Readonly<{ row: SenderDecision }>) {
  const t = useT();
  const { locale } = useLocale();
  const zone = viewerZone();
  const kind = row.kind ?? "";
  const words = kindLabel[kind] ? t(kindLabel[kind]) : kind;
  if (row.overruled) {
    return (
      <>
        <Badge tone={row.decision === "business" ? "success" : "danger"}>
          {row.decision === "business"
            ? t("senders.kind.business")
            : t("senders.kind.keptOut")}
        </Badge>{" "}
        {t("senders.byYou")}
      </>
    );
  }
  return (
    <>
      <Badge tone={admitted.has(kind) ? "success" : undefined} quiet>
        {words || t("senders.kind.undecided")}
      </Badge>
      {/* The deadline, not just the verdict. A personal verdict does not hide
          this sender's mail, so during the window nothing on the page looks
          different and there is nothing for an owner to object to — the date is
          what turns a silent classification into something they can act on, and
          "mark as business" beside it is the act that cancels it. */}
      {row.deletes_at && (
        <div className="cell-note">
          {t("senders.deletesOn", {
            date: formatDate(row.deletes_at, locale, zone),
          })}
        </div>
      )}
    </>
  );
}
