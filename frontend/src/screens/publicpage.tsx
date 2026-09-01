import type { ReactNode } from "react";
import { Logomark } from "../design-system/logomark";
import { useT } from "../i18n";
import { usePublicLocale } from "../i18n/publiclocale";
import "./preferences.css";

/**
 * The frame both public pages sit in.
 *
 * They are reached from one message by one reader, so they have to look
 * like one product: the same column width, the same ground, the same
 * sign-off underneath. Before this each page centred itself — or, in the
 * unsubscribe page's case, forgot to, and its content spread across the
 * whole window because `.pref-page` is a centring flex row with nothing
 * to centre.
 *
 * It also carries the language the link asked for, so neither page has to
 * remember to.
 */
export function PublicPage({ children }: Readonly<{ children: ReactNode }>) {
  usePublicLocale();
  return (
    <div className="pref-page">
      <div className="pref-center arrive-stack">
        {children}
        <PublicFooter />
      </div>
    </div>
  );
}

/**
 * Who sent this.
 *
 * A page that asks somebody to make a decision about their own mail owes
 * them the name of the product asking — otherwise an unbranded page
 * arriving from a link is indistinguishable from a phishing attempt.
 */
function PublicFooter() {
  const t = useT();
  return (
    <p className="public-foot">
      <Logomark size={15} />
      {t("prefs.sentVia")}
    </p>
  );
}
