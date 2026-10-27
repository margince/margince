import { useQuery } from "@tanstack/react-query";
import { ArrowLeft, PenLine } from "lucide-react";
import { useState } from "react";
import { api } from "../../api/client";
import type { components } from "../../api/schema";
import { Badge, Button } from "../../design-system/atoms";
import { Panel, PanelBody } from "../../design-system/panel";
import { QueryGate } from "../../design-system/querystates";
import { formatMoney, formatNumber } from "../../format/format";
import { offerStatusLabel } from "../../format/offerstatus";
import { type Locale, useLocale, usePlural, useT } from "../../i18n";
import { unwrap } from "../../lib/problem";
import { navigate } from "../../lib/router";
import { OfferTotalsPanel } from "./offerrecurring";
import "./offers.css";

import {
  AcceptOfferAction,
  RegenerateOfferAction,
  RejectOfferAction,
  RenderOfferPdfAction,
  SendOfferAction,
} from "./offers.actions";
// The offer 360: header, read-only totals, and a draft-only header edit whose
// surface is its own because buyer_company_id needs the shared RecordPicker and
// template_id a server-sourced select — EditAction/CreateField fits neither.
import { EditOfferHeaderModal } from "./offers.header";
import { OfferLineEditor } from "./offers.lines";

export type Offer = components["schemas"]["Offer"];

export type OfferTemplate = components["schemas"]["OfferTemplate"];

export type OfferLineItem = components["schemas"]["OfferLineItem"];

export type OfferLineItemInput = components["schemas"]["OfferLineItemInput"];

export type UpdateOfferLineItemRequest =
  components["schemas"]["UpdateOfferLineItemRequest"];

// The AI provenance notice and the diff-from-previous summary. Both fields
// are filled only on the regenerate response that produced this draft. The
// offer here may be a later read (ai_generated back to false). So the banner
// renders nothing, not an empty shell, whenever ai_generated is not true. The
// disclosure text is a compliance-mandated string: rendered verbatim, never
// reworded or wrapped in translated copy.
function DiffLine({
  line,
  currency,
  locale,
}: Readonly<{ line: OfferLineItem; currency: string; locale: Locale }>) {
  return (
    <li>
      <span>{line.description}</span>
      {line.price_grounded === false ? null : (
        <>
          {" — "}
          <span className="t-num">
            {formatMoney(line.line_total_minor, currency, locale)}
          </span>
        </>
      )}
    </li>
  );
}

function AiDisclosureBanner({ offer }: Readonly<{ offer: Offer }>) {
  const t = useT();
  const plural = usePlural();
  const { locale } = useLocale();
  if (!offer.ai_generated) {
    return null;
  }
  const diff = offer.diff_from_previous;
  const added = diff?.added ?? [];
  const removed = diff?.removed ?? [];
  const changed = diff?.changed ?? [];

  return (
    // No tone and no badge: indigo claims authorship, and this panel reports
    // on a machine's work in the product's own words, offering no AI verb.
    <Panel title={t("offer.aiDisclosureTitle")}>
      <PanelBody>
        {offer.ai_disclosure && <p className="t-body">{offer.ai_disclosure}</p>}
        {diff && (
          <div data-testid="offer-diff-summary" className="offers-diff-summary">
            {added.length > 0 && (
              <div>
                <p className="t-label">
                  {plural("offer.diffAdded", added.length, {
                    count: formatNumber(added.length, locale),
                  })}
                </p>
                <ul>
                  {added.map((line) => (
                    <DiffLine
                      key={line.id}
                      line={line}
                      currency={offer.currency}
                      locale={locale}
                    />
                  ))}
                </ul>
              </div>
            )}
            {removed.length > 0 && (
              <div>
                <p className="t-label">
                  {plural("offer.diffRemoved", removed.length, {
                    count: formatNumber(removed.length, locale),
                  })}
                </p>
                <ul>
                  {removed.map((line) => (
                    <DiffLine
                      key={line.id}
                      line={line}
                      currency={offer.currency}
                      locale={locale}
                    />
                  ))}
                </ul>
              </div>
            )}
            {changed.length > 0 && (
              <div>
                <p className="t-label">
                  {plural("offer.diffChanged", changed.length, {
                    count: formatNumber(changed.length, locale),
                  })}
                </p>
                <ul>
                  {changed.map((pair) =>
                    pair.after ? (
                      <DiffLine
                        key={pair.after.id}
                        line={pair.after}
                        currency={offer.currency}
                        locale={locale}
                      />
                    ) : null,
                  )}
                </ul>
              </div>
            )}
          </div>
        )}
      </PanelBody>
    </Panel>
  );
}

export function OfferScreen({ id }: Readonly<{ id: string }>) {
  const t = useT();
  const [editing, setEditing] = useState(false);
  const offerQuery = useQuery({
    queryKey: ["offer", id],
    queryFn: async () => {
      return unwrap(
        await api.GET("/offers/{id}", {
          params: { path: { id } },
        }),
      );
    },
    // RegenerateOfferAction seeds this exact key with its 201 response — the
    // only place ai_disclosure/diff_from_previous are ever populated — right
    // before navigating here, and the default refetchOnMount would re-GET and
    // wipe both the instant this screen mounts. A genuinely fresh navigation
    // is unaffected: refetchOnMount only skips when the key is already cached.
    refetchOnMount: false,
  });

  return (
    <div className="wrap">
      <QueryGate query={offerQuery} pendingLabel={t("nav.offers")}>
        {(offer) => (
          <>
            {/* The status is a fact about the record; the lifecycle verbs
                change it, so they take the band rather than the head. */}
            <Panel
              title={offer.offer_number}
              titleAction={<Badge>{offerStatusLabel(offer.status, t)}</Badge>}
              actions={
                <>
                  <Button
                    onClick={() =>
                      navigate({ screen: "deals", id: offer.deal_id })
                    }
                  >
                    <ArrowLeft aria-hidden /> {t("offer.backToDeal")}
                  </Button>
                  {offer.status === "draft" && (
                    <Button
                      data-testid="edit-offer-header"
                      onClick={() => setEditing(true)}
                    >
                      <PenLine aria-hidden /> {t("offer.edit")}
                    </Button>
                  )}
                  {offer.status === "draft" && (
                    <SendOfferAction offer={offer} />
                  )}
                  {offer.status === "sent" && (
                    <>
                      <AcceptOfferAction offer={offer} />
                      <RejectOfferAction offer={offer} />
                      <RegenerateOfferAction offer={offer} />
                    </>
                  )}
                </>
              }
            >
              {null}
            </Panel>
            <AiDisclosureBanner offer={offer} />
            <OfferTotalsPanel offer={offer} />
            <RenderOfferPdfAction offer={offer} />
            {offer.status === "draft" && <OfferLineEditor offer={offer} />}
            {offer.status === "draft" && (
              <EditOfferHeaderModal
                open={editing}
                onClose={() => setEditing(false)}
                offer={offer}
              />
            )}
          </>
        )}
      </QueryGate>
    </div>
  );
}
