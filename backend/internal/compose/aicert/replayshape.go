// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

// What a journaled answer was an answer to. A replay hands a case the
// journaled answer only for a request of the same shape. So the shape holds
// every request field that can change what a model says.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// replayRouting versions what the router and the provider adapters do to a
// request after the case builds it, which no request shape can see. Bump it
// with a change there that can change an answer: every journaled run is then
// paid for again.
const replayRouting = "replay-routing-1"

// idTable numbers per-call row ids in the order a run's requests first carry them.
type idTable struct {
	ids   []string
	index map[string]int
}

func (t *idTable) ordinal(id string) int {
	if at, seen := t.index[id]; seen {
		return at
	}
	if t.index == nil {
		t.index = map[string]int{}
	}
	t.index[id] = len(t.ids)
	t.ids = append(t.ids, id)
	return t.index[id]
}

// answerSettings are the request fields outside the prompt that still decide
// the answer. They name the model and site, how hard it thinks, its company
// context and attachments, and each vendor's own knobs.
//
// The stamp leaves them out. The router fills several of them, so a stamp
// holding them could not tell a scenario edit from a routing one.
type answerSettings struct {
	Model                  string                     `json:"model"`
	Site                   string                     `json:"site"`
	ThinkingFloor          string                     `json:"thinking_floor"`
	ThinkingLevel          string                     `json:"thinking_level"`
	ContextScopes          []string                   `json:"context_scopes"`
	ContextFingerprint     string                     `json:"context_fingerprint"`
	IncludeCompanyContext  bool                       `json:"include_company_context"`
	CompanyContextOffering string                     `json:"company_context_offering"`
	ProviderOptions        map[string]json.RawMessage `json:"provider_options"`
	Attachments            []attachmentShape          `json:"attachments"`
}

// attachmentShape is an attachment by digest, so a journal line never holds a
// file's bytes.
type attachmentShape struct {
	MIME, Name, URI, Digest string
}

func answerSettingsOf(req model.Request) answerSettings {
	settings := answerSettings{
		Model: req.Model, Site: req.Site, ThinkingFloor: req.ThinkingFloor, ThinkingLevel: req.ThinkingLevel,
		ContextScopes: req.ContextScopes, ContextFingerprint: req.ContextFingerprint,
		IncludeCompanyContext: req.IncludeCompanyContext, CompanyContextOffering: req.CompanyContextOffering,
		ProviderOptions: req.ProviderOptions,
	}
	for _, a := range req.Attachments {
		sum := sha256.Sum256(a.Bytes)
		settings.Attachments = append(settings.Attachments,
			attachmentShape{MIME: a.MIME, Name: a.Name, URI: a.URI, Digest: hex.EncodeToString(sum[:])})
	}
	return settings
}

// requestShape digests a request with each row id replaced by its ordinal in
// table. Two requests share a shape only when they differ in nothing but which
// ids were minted, and mint them in the same places.
func requestShape(req model.Request, table *idTable) (string, error) {
	prompt, err := canonicalRequestMaterial(req)
	if err != nil {
		return "", err
	}
	settings, err := json.Marshal(answerSettingsOf(req))
	if err != nil {
		return "", fmt.Errorf("aicert: the request's settings cannot be digested: %w", err)
	}
	material := append(append(prompt, '\n'), settings...)
	renamed := perCallID.ReplaceAllFunc(material, func(id []byte) []byte {
		return []byte("id-" + strconv.Itoa(table.ordinal(string(id))))
	})
	sum := sha256.Sum256([]byte(promptfence.ReplaceNonces(string(renamed), canonicalID)))
	return hex.EncodeToString(sum[:]), nil
}
