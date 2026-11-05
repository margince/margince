// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestDocumentExplanationsCoverEveryLanguage(t *testing.T) {
	langcopytest.Census(t, documentWords)
}

func TestDocumentLengthExplainsCharactersRatherThanBytes(t *testing.T) {
	text := strings.Repeat("ü", maxDocumentTextChars+1)
	for _, language := range []string{"en", "de", "vi"} {
		reader := &DocumentExtractor{said: documentCopyFor(language)}
		source, detail := reader.textSource(textAttachment("text/plain"), []byte(text))
		if source.Text != "" || !strings.Contains(detail, fmt.Sprint(maxDocumentTextChars+1)) || strings.Contains(detail, fmt.Sprint(len(text))) {
			t.Fatalf("%s misreports the character bound: %s", language, detail)
		}
	}
}
