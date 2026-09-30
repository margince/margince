// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Which sent documents Deal Scout reads as a proposal or a contract. The
// decision is the file's type and the WORDS of its name, never a substring: a
// "Newsletter_Angebote.pdf" carries the word newsletter, and "Vertragsnummer"
// inside a longer word is not a contract.
//
// The matcher is SQL, so the scout can tell which companies qualify before it
// caps how many it reads. The vocabularies stay here, in one place, and reach
// the statement as bound arrays.

import (
	"fmt"
	"strings"
)

// proposalWords are the file-name words that name a proposal, a quote or a
// contract, in German, English and Vietnamese, written without accents
// because names are compared after folding them away. A two-word entry
// matches two neighbouring words.
//
//nolint:goconst // these are words a file name may carry; the constants goconst points at name record types that happen to be spelled the same
var proposalWords = []string{
	"angebot", "kostenvoranschlag", "vertrag", "rahmenvertrag", "vertragsentwurf",
	"proposal", "quote", "quotation", "offer", "contract", "agreement", "sow", "msa",
	"statement of work",
	"bao gia", "baogia", "hop dong", "hopdong",
}

// A file that names a bill or a piece of marketing is not a proposal, whatever
// else its name says: these words outrank proposalWords.
var (
	billingWords   = []string{"invoice", "rechnung", "receipt", "quittung", "gutschrift", "hoa don", "hoadon", "bien lai"}
	marketingWords = []string{"newsletter", "brochure", "broschure", "flyer", "catalog", "katalog"}
)

// documentTypes are the content types a proposal arrives as. Images are left
// out on purpose: an image on an outbound mail is almost always a logo or a
// signature part.
var documentTypes = []string{
	"application/pdf", "application/msword", "application/rtf",
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	"application/vnd.openxmlformats-officedocument.presentationml.presentation",
	"application/vnd.ms-excel", "application/vnd.ms-powerpoint",
	"application/vnd.oasis.opendocument.text",
	"application/vnd.oasis.opendocument.spreadsheet",
	"application/vnd.oasis.opendocument.presentation",
}

// documentExtensions stand in for a content type the provider did not send.
var documentExtensions = []string{
	".pdf", ".doc", ".docx", ".rtf", ".odt", ".xls", ".xlsx", ".ods", ".ppt", ".pptx", ".odp",
}

// filenameWordsSQL folds a file name's stem to lower-case ASCII words framed by
// spaces: accents are dropped after NFD decomposition (the Vietnamese đ is a
// letter of its own and is mapped by hand), and every separator and every
// change between letters and digits splits a word, so "Angebot2026_v2.pdf"
// reads " angebot 2026 v 2 ".
func filenameWordsSQL(filename string) string {
	stem := `regexp_replace(` + filename + `, '\.[^./]*$', '')`
	folded := `lower(translate(regexp_replace(normalize(` + stem + `, NFD), '[̀-ͯ]', '', 'g'), 'đĐ', 'dd'))`
	split := `regexp_replace(regexp_replace(` + folded + `, '([[:alpha:]])([[:digit:]])', '\1 \2', 'g'), '([[:digit:]])([[:alpha:]])', '\1 \2', 'g')`
	return `(' ' || btrim(regexp_replace(` + split + `, '[^[:alnum:]]+', ' ', 'g')) || ' ')`
}

// proposalDocumentSQL is the condition that an attachment is a document whose
// name says it is a proposal or a contract. arg binds the vocabularies.
func proposalDocumentSQL(filename, contentType string, arg func(any) int) string {
	words := filenameWordsSQL(filename)
	bareType := `lower(btrim(split_part(coalesce(` + contentType + `, ''), ';', 1)))`
	ext := `lower(coalesce(substring(` + filename + ` from '\.[^./]*$'), ''))`
	outranking := append(append([]string{}, billingWords...), marketingWords...)
	return fmt.Sprintf(`((%[1]s = ANY($%[3]d) OR (%[1]s = '' AND %[2]s = ANY($%[4]d)))
	   AND %[5]s LIKE ANY($%[6]d) AND NOT %[5]s LIKE ANY($%[7]d))`,
		bareType, ext, arg(documentTypes), arg(documentExtensions),
		words, arg(likeWords(proposalWords)), arg(likeWords(outranking)))
}

// likeWords frames each vocabulary entry as a whole-word LIKE pattern.
func likeWords(words []string) []string {
	out := make([]string, 0, len(words))
	for _, word := range words {
		out = append(out, "% "+strings.TrimSpace(word)+" %")
	}
	return out
}
