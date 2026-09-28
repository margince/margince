// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Which sent documents Deal Scout reads as a proposal or a contract. The
// decision is the file's type and the WORDS of its name, never a substring: a
// "Newsletter_Angebote.pdf" carries the word newsletter, and "Vertragsnummer"
// inside a longer word is not a contract.

import (
	"path"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
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
var documentTypes = map[string]bool{
	"application/pdf":    true,
	"application/msword": true,
	"application/rtf":    true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
	"application/vnd.ms-excel":                        true,
	"application/vnd.ms-powerpoint":                   true,
	"application/vnd.oasis.opendocument.text":         true,
	"application/vnd.oasis.opendocument.spreadsheet":  true,
	"application/vnd.oasis.opendocument.presentation": true,
}

// documentExtensions stand in for a content type the provider did not send.
var documentExtensions = map[string]bool{
	".pdf": true, ".doc": true, ".docx": true, ".rtf": true, ".odt": true,
	".xls": true, ".xlsx": true, ".ods": true, ".ppt": true, ".pptx": true, ".odp": true,
}

// isProposalDocument reports whether an attachment is a document whose name
// says it is a proposal or a contract.
func isProposalDocument(filename, contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(strings.SplitN(contentType, ";", 2)[0]))
	ext := strings.ToLower(path.Ext(filename))
	if !documentTypes[contentType] && (contentType != "" || !documentExtensions[ext]) {
		return false
	}
	words := " " + strings.Join(filenameWords(strings.TrimSuffix(filename, path.Ext(filename))), " ") + " "
	return !namesAny(words, billingWords) && !namesAny(words, marketingWords) && namesAny(words, proposalWords)
}

// namesAny reports whether the space-framed word string holds any of the
// words, a two-word entry as two neighbours.
func namesAny(words string, vocabulary []string) bool {
	for _, word := range vocabulary {
		if strings.Contains(words, " "+word+" ") {
			return true
		}
	}
	return false
}

// filenameWords folds a name to lower-case letters without accents and splits
// it into words at every separator and at each change between letters and
// digits, so "Angebot2026_v2" is angebot, 2026, v, 2.
func filenameWords(name string) []string {
	var words []string
	var current []rune
	lastDigit := false
	flush := func() {
		if len(current) > 0 {
			words = append(words, string(current))
			current = current[:0]
		}
	}
	for _, r := range foldAccents(name) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			digit := unicode.IsDigit(r)
			if len(current) > 0 && digit != lastDigit {
				flush()
			}
			current = append(current, unicode.ToLower(r))
			lastDigit = digit
		default:
			flush()
		}
	}
	flush()
	return words
}

// foldAccents drops combining marks, so "Báo giá" reads as "Bao gia" and
// "Broschüre" as "Broschure". The Vietnamese đ is a letter of its own rather
// than a d with a mark, so it is mapped by hand.
func foldAccents(s string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(s) {
		switch {
		case unicode.Is(unicode.Mn, r):
			continue
		case r == 'đ':
			b.WriteRune('d')
		case r == 'Đ':
			b.WriteRune('D')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}
