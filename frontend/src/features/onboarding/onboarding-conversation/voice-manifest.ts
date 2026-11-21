import type { components } from "../../../api/schema";
import { formatNumber } from "../../../format/format";
import type { Locale, PluralTranslator } from "../../../i18n";

type CorpusSource = components["schemas"]["VoiceCorpusSource"];

export type CorpusManifestEntry = Readonly<{
  ref: string;
  /** The server row and its version, which a removal names. */
  id: string;
  version: number;
  label: string;
  /** Server-counted words that survived the speaker filter. */
  keptWords: number;
  /** Server-counted words of the whole source before filtering. */
  inputWords: number;
  /** Kept-of-total is shown only where filtering actually discarded turns. */
  transcript: boolean;
  /** Sentences of the reader's own prose, for the distilling panel to read
   * back. Empty for a transcript: its text carries other speakers' turns, and
   * which words are the reader's is decided on the server, not here. */
  lines: readonly string[];
}>;

/** The count a Sources row shows; kept-of-total only for a filtered transcript. */
export function manifestCount(
  entry: CorpusManifestEntry,
  plural: PluralTranslator,
  locale: Locale,
): string {
  const kept = formatNumber(entry.keptWords, locale);
  if (entry.transcript) {
    return plural("ob.conv.voice.manifestKept", entry.inputWords, {
      kept,
      total: formatNumber(entry.inputWords, locale),
    });
  }
  return plural("ob.conv.voice.manifestWords", entry.keptWords, {
    words: kept,
  });
}

// A stored row carries no pre-filter count and no text. It lists as its kept
// words alone and lends no lines to the distilling panel.
export function storedEntry(source: CorpusSource): CorpusManifestEntry {
  return {
    ref: source.source_ref,
    id: source.id,
    version: source.version,
    label: source.source_label,
    keptWords: source.word_count,
    inputWords: source.word_count,
    transcript: false,
    lines: [],
  };
}
