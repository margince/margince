import {
  type InputHTMLAttributes,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import {
  type AmountRefusal,
  typedAmountText,
  typedMinorUnits,
} from "../format/minorunits";
import { type Locale, useLocale, useT } from "../i18n";
import { TextInput } from "./atoms";
import { ErrorLine } from "./errorline";

// A thin wrapper around a text TextInput that displays and edits MAJOR units
// while emitting the MINOR units the API stores. Text, not type="number": a
// number box reads the browser's decimal sign, so "12,5" became 125. The text
// is read by typedMinorUnits, which refuses an ambiguous "1.500".
//
// The scale is the CURRENCY's, taken from format/minorunits. This used to
// assume two decimals, on the stated grounds that no caller needed a different
// count — which was true of the callers and false of the currencies: an offer
// priced in dong went to the server multiplied by a hundred, and the server had
// no way to tell that figure from a real one. So `currency` is required rather
// than defaulted; a component that silently assumes EUR is the same bug with a
// friendlier signature.
//
// The displayed text is its OWN state, not `(valueMinor / 100).toFixed(2)`
// recomputed on every render: a fully-derived display reformats after every
// keystroke (typing "1" then "2" for "12.50" renders "1.00" after the first
// keystroke, so the next keystroke lands on the already-rounded string
// instead of the one the user meant to extend). `lastCommittedMinor` tracks
// which minor value this input itself last emitted, so the resync effect
// below only snaps the text to the external value when it changes for a
// reason OTHER than this input's own typing (a different row's value
// swapped in, a reset) — never mid-edit.
export function MoneyInput({
  valueMinor,
  currency,
  onChangeMinor,
  onBlur,
  blankWhenZero = false,
  onClear,
  display,
  onFocus,
  onRefusedChange,
  ...rest
}: Readonly<
  Omit<InputHTMLAttributes<HTMLInputElement>, "value" | "onChange" | "type"> & {
    valueMinor: number;
    currency: string;
    onChangeMinor: (minor: number) => void;
    // Show an unpriced record as an EMPTY field rather than "0.00".
    //
    // Off by default, because an offer line always has a price and "0.00" is
    // the honest reading of a free one. On for a record that may simply not be
    // priced yet, where a pre-filled zero is a figure nobody entered — and one
    // a reader then types after, turning 5000 into 0.005000.
    blankWhenZero?: boolean;
    // Called when the reader empties the box, which then stays empty on blur.
    //
    // Opt-in, for a value that may legitimately be absent — a filter operand
    // the reader is taking back. Without it an emptied box keeps the last
    // amount, which is right for a priced record (see onChange below).
    onClear?: () => void;
    // The amount as the page writes money, shown while the box is not being
    // edited. Focus brings back the plain number, which is what a reader types.
    display?: (minor: number) => string;
    // Told whether the box holds text it refuses, for a save outside a form
    // or dialog, where refuseUnreadableField cannot reach the box.
    onRefusedChange?: (refused: boolean) => void;
  }
>) {
  const t = useT();
  const { locale } = useLocale();
  const errorId = useId();
  const asText = (minor: number, forCurrency: string) =>
    blankWhenZero && minor === 0
      ? ""
      : typedAmountText(minor, forCurrency, locale);
  const [text, setText] = useState(() => asText(valueMinor, currency));
  const [editing, setEditing] = useState(false);
  // Why the text on screen is not the amount, shown once the reader leaves
  // the box; the text itself stays so they can correct it.
  const [refusal, setRefusal] = useState<AmountRefusal | null>(null);
  const lastCommittedMinor = useRef(valueMinor);
  // Whether the reader's last edit emptied the box through onClear, so blur
  // leaves it empty rather than restoring the amount it held.
  const cleared = useRef(false);
  // The amount the box held when this edit began. A refused text puts it
  // back, so a prefix committed key by key ("1.5" of "1.500") is never saved.
  const beforeEdit = useRef<{ minor: number; cleared: boolean } | null>(null);
  // The currency the text on screen is written in, tracked beside the amount
  // because the SCALE is part of what that text says: an offer switched from
  // EUR to VND holds the same minor integer and must not keep showing the euro
  // reading of it.
  const lastRenderedCurrency = useRef(currency);

  useEffect(() => {
    // Object.is, not ===: a refused amount travels as NaN, and NaN === NaN is
    // false, so the parent echoing our own NaN back read as an external change
    // and snapped the buffer mid-edit — the one thing this guard exists to
    // prevent.
    if (
      Object.is(valueMinor, lastCommittedMinor.current) &&
      currency === lastRenderedCurrency.current
    ) {
      return;
    }
    // Inlined rather than through asText: a function created each render is a
    // dependency this effect must not have — listing it re-runs the resync on
    // every render and snaps the text mid-edit, which is the whole thing
    // lastCommittedMinor exists to prevent.
    setText(
      blankWhenZero && valueMinor === 0
        ? ""
        : typedAmountText(valueMinor, currency, locale),
    );
    setRefusal(null);
    lastCommittedMinor.current = valueMinor;
    lastRenderedCurrency.current = currency;
  }, [valueMinor, currency, blankWhenZero, locale]);

  // The refusal the text on screen earns now, before any blur shows it. It is
  // the box's custom validity, so refuseUnreadableField stops a save past it.
  // No currency, no scale: the box takes nothing until one arrives. A keystroke
  // is never committed at a guessed scale and redrawn at the real one.
  const noCurrency = currency.trim() === "";
  const typedNow =
    text.trim() === "" || noCurrency
      ? null
      : typedMinorUnits(text, currency, locale);
  const pending = typedNow && "refused" in typedNow ? typedNow.refused : null;
  const pendingMessage = pending ? t(pending.key, pending.params) : "";
  const input = useRef<HTMLInputElement | null>(null);
  // Every render: the face that holds the input changes with editing.
  useEffect(() => {
    input.current?.setCustomValidity(pendingMessage);
  });
  const refusedNow = pending !== null;
  useEffect(() => {
    onRefusedChange?.(refusedNow);
  }, [refusedNow, onRefusedChange]);

  const startEdit = () => {
    setEditing(true);
    beforeEdit.current ??= {
      minor: lastCommittedMinor.current,
      cleared: cleared.current,
    };
  };
  const commit = (minor: number) => {
    cleared.current = false;
    lastCommittedMinor.current = minor;
    onChangeMinor(minor);
  };
  const restoreBeforeEdit = () => {
    const before = beforeEdit.current;
    if (!before) {
      return;
    }
    if (before.cleared && onClear) {
      cleared.current = true;
      onClear();
    } else if (!Object.is(before.minor, lastCommittedMinor.current)) {
      commit(before.minor);
    }
  };
  if (noCurrency) {
    return (
      <TextInput
        {...rest}
        ref={input}
        type="text"
        inputMode="decimal"
        value=""
        readOnly
        disabled
        placeholder={t("amount.waitsForCurrency")}
      />
    );
  }
  if (display && !editing && text !== "" && refusal === null) {
    return (
      <TextInput
        {...rest}
        type="text"
        inputMode="decimal"
        value={display(lastShownMinor(text, valueMinor, currency, locale))}
        // Only focus brings a key press here, and focus has already swapped
        // this face for the editor.
        onChange={() => setEditing(true)}
        onFocus={(event) => {
          startEdit();
          onFocus?.(event);
        }}
      />
    );
  }
  const describedBy = [rest["aria-describedby"], refusal ? errorId : undefined]
    .filter(Boolean)
    .join(" ");
  return (
    <>
      <TextInput
        ref={input}
        type="text"
        inputMode="decimal"
        value={text}
        onFocus={(event) => {
          startEdit();
          onFocus?.(event);
        }}
        onChange={(event) => {
          // A box that started empty has no display face to focus through.
          startEdit();
          setRefusal(null);
          setText(event.target.value);
          // An empty buffer is never committed as 0: the last amount stands,
          // so a priced record cannot be emptied by deleting its text. A
          // value that may be absent takes onClear instead.
          if (event.target.value.trim() === "") {
            if (onClear) {
              cleared.current = true;
              onClear();
            }
            return;
          }
          // A refused text puts back the amount the edit began with, at once.
          // A prefix committed key by key ("1.5" of "1.500") then never
          // reaches a caller that saves on blur.
          const typed = typedMinorUnits(event.target.value, currency, locale);
          if ("minor" in typed) {
            commit(typed.minor);
          } else {
            restoreBeforeEdit();
          }
        }}
        onInvalid={() => setRefusal(pending)}
        onBlur={(event) => {
          if (pending) {
            setRefusal(pending);
          } else {
            setText(
              cleared.current
                ? ""
                : asText(lastCommittedMinor.current, currency),
            );
          }
          beforeEdit.current = null;
          setEditing(false);
          onBlur?.(event);
        }}
        {...rest}
        aria-invalid={refusal ? true : rest["aria-invalid"]}
        aria-describedby={describedBy || undefined}
      />
      {refusal && (
        <ErrorLine id={errorId}>{t(refusal.key, refusal.params)}</ErrorLine>
      )}
    </>
  );
}

// The amount the display form names: what the text on screen says, so a
// figure held mid-edit is never shown as the stored one.
function lastShownMinor(
  text: string,
  valueMinor: number,
  currency: string,
  locale: Locale,
) {
  const typed = typedMinorUnits(text, currency, locale);
  return "minor" in typed ? typed.minor : valueMinor;
}
