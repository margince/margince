import { useMutation } from "@tanstack/react-query";
import { ArrowLeft } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import {
  Badge,
  Button,
  Card,
  Field,
  SectionHeader,
  TextInput,
} from "../design-system/atoms";
import { ErrorLine } from "../design-system/errorline";
import { Row } from "../design-system/stack";
import { useT } from "../i18n";
import { throwProblem } from "./common";
import "./client.css";

// Client surfaces (B-EP09.13a): the rail-less extension chrome — the fixed
// dark "Back to Margince" bar, a sender lookup that renders a mini-360 for a
// recognized contact and the HONEST unknown-sender state otherwise, and the
// load-bearing isolation footer: this surface talks only to the user's OWN
// workspace API (S-E12.5) — no third-party egress.

export function ClientSurfaceScreen() {
  const t = useT();
  const [email, setEmail] = useState("");

  const lookup = useMutation({
    mutationFn: async (query: string) => {
      const { data, error } = await api.GET("/search", {
        params: { query: { q: query, limit: 5 } },
      });
      if (error) {
        throwProblem(error);
      }
      return data.data.filter((hit) => hit.type === "contact");
    },
  });

  return (
    <div className="client-chrome">
      <header className="client-bar">
        <a href="#/home" className="client-back">
          <ArrowLeft aria-hidden size={15} />
          {t("client.back")}
        </a>
      </header>
      <div className="wrap narrow">
        <SectionHeader title={t("client.title")} />
        <div className="client-lookup">
          <Field label={t("client.sender")}>
            {(control) => (
              <TextInput
                {...control}
                value={email}
                onChange={(event) => setEmail(event.target.value)}
              />
            )}
          </Field>
          <Button
            variant="primary"
            disabled={email.trim() === "" || lookup.isPending}
            onClick={() => lookup.mutate(email.trim())}
          >
            {t("client.lookup")}
          </Button>
        </div>

        {lookup.isSuccess && lookup.data.length > 0 && (
          <Card className="clientsurface-answer">
            {lookup.data.map((hit) => (
              <Row key={hit.id} gap="2" wrap={false}>
                <strong>{hit.title}</strong>
                {hit.snippet && (
                  <span className="t-caption">{hit.snippet}</span>
                )}
                <a href={`#/contacts/${hit.id}`}>{t("client.open360")}</a>
              </Row>
            ))}
          </Card>
        )}

        {lookup.isSuccess && lookup.data.length === 0 && (
          <Card inset className="clientsurface-answer">
            <p>{t("client.unknown")}</p>
            <p className="clientsurface-unknown-detail">
              {t("client.unknownDetail")}
            </p>
            <div className="card-actions">
              <a className="btn btn-ghost" href="#/leads">
                {t("client.createLead")}
              </a>
            </div>
          </Card>
        )}

        <ErrorLine error={lookup.error} />

        <footer className="client-footer">
          <Badge>{t("client.isolation")}</Badge>
          <span className="t-caption">{t("client.attribution")}</span>
        </footer>
      </div>
    </div>
  );
}
