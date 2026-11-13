// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

// What Margince writes into a linked channel: the card, and one post per
// flush with a line per event. The text is built from the live record and
// from sharedActivity, never from a stored copy.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/labeltranslation"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// chatRecord is the record a card shows: the deal when the link has one,
// otherwise the project.
type chatRecord struct {
	kind       string
	id         ids.UUID
	name       string
	stage      string
	status     string
	due        *time.Time
	ownerName  string
	ownerEmail string
}

// readRecord reads what a channel shows of its record: the deal, or the
// project when the link carries no deal.
func readRecord(ctx context.Context, pool *pgxpool.Pool, link chatlinks.Link) (chatRecord, error) {
	record := chatRecord{kind: chatlinks.RecordProject}
	args := []any{nil}
	arg := func(v any) int { args = append(args, v); return len(args) }
	query := `SELECT p.name, p.phase, '', p.target_end_date::timestamptz,
		coalesce(u.display_name, ''), coalesce(u.email, '')
		FROM project p LEFT JOIN app_user u ON u.id = p.owner_id WHERE p.id = $1`
	switch {
	case link.DealID != nil:
		record.kind, record.id = chatlinks.RecordDeal, *link.DealID
		// The channel is written in English, so the stage reads in English.
		stage := labeltranslation.SQLIn(channelLanguage, "s.name", "s.name_translations", arg)
		query = `SELECT d.name, coalesce(` + stage + `, ''), d.status, d.expected_close_date::timestamptz,
			coalesce(u.display_name, ''), coalesce(u.email, '')
			FROM deal d LEFT JOIN stage s ON s.id = d.stage_id LEFT JOIN app_user u ON u.id = d.owner_id
			WHERE d.id = $1`
	case link.ProjectID != nil:
		record.id = *link.ProjectID
	}
	args[0] = record.id
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, args...).Scan(&record.name, &record.stage, &record.status, &record.due,
			&record.ownerName, &record.ownerEmail)
	})
	return record, err
}

// channelLanguage is the language Margince writes a channel in.
const channelLanguage = "en"

// chatSignature closes every bot text, so a reader knows who wrote it and
// where the record lives.
func chatSignature(record chatRecord, origin string) string {
	return "Written by Margince · " + mailcopy.Destination(origin, mailcopy.RecordFragment(record.kind, record.id.String()))
}

// chatCardText is the card: what the record is and where it stands. The
// amount stays out: a channel is read by everyone in it, and the amount is
// masked from members who may not see it.
func chatCardText(record chatRecord, origin string) string {
	var facts []string
	if record.stage != "" {
		facts = append(facts, record.stage)
	}
	if record.status != "" && record.status != "open" {
		facts = append(facts, record.status)
	}
	if record.due != nil {
		label := "Close"
		if record.kind == chatlinks.RecordProject {
			label = "Target end"
		}
		facts = append(facts, label+" "+record.due.Format("2006-01-02"))
	}
	if record.ownerEmail != "" {
		facts = append(facts, "Owner "+mention(record.ownerEmail, record.ownerName))
	}
	lines := []string{"*" + strings.TrimSpace(record.name) + "*"}
	if len(facts) > 0 {
		lines = append(lines, strings.Join(facts, " · "))
	}
	return strings.Join(append(lines, chatSignature(record, origin)), "\n")
}

// chatPostText is one flush: the record's name, a line per event, and the
// signature.
func chatPostText(record chatRecord, lines []string, origin string) string {
	if len(lines) == 0 {
		return "An update here is no longer shared.\n" + chatSignature(record, origin)
	}
	var b strings.Builder
	b.WriteString(strings.TrimSpace(record.name))
	b.WriteString("\n")
	for _, line := range lines {
		b.WriteString("• " + line + "\n")
	}
	b.WriteString(chatSignature(record, origin))
	return b.String()
}

func mention(email, name string) string {
	if name == "" {
		name = email
	}
	return "<@" + email + "|" + name + ">"
}

func chatFingerprint(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:16])
}

// chatItemLine is the line one event adds to a post.
func chatItemLine(item chatlinks.Item, shared sharedText) string {
	if item.ActivityID != nil {
		return activityLine(item, shared)
	}
	detail := func(key string) string {
		value, _ := item.Detail[key].(string)
		return value
	}
	switch item.EventType {
	case "deal.stage_changed":
		line := "Stage moved to " + detail("to_stage_name")
		if status := detail("to_status"); status == "won" || status == "lost" {
			line += " (" + status + ")"
		}
		return line
	case "project.phase_changed":
		return "Phase moved to " + detail("to_phase")
	case "contract.status_changed":
		return "Contract is now " + detail("status")
	case "deal.updated", "project.updated":
		return "Updated: " + changedFieldWords(item.Detail)
	}
	if line, ok := chatEventLines[item.EventType]; ok {
		return line
	}
	return strings.ReplaceAll(item.EventType, "_", " ")
}

var chatEventLines = map[string]string{
	"deal.archived":      "Deal archived",
	"deal.restored":      "Deal restored",
	"deal.owner_changed": "Owner changed",
	"offer.created":      "Offer drafted",
	"offer.updated":      "Offer edited",
	"offer.sent":         "Offer sent",
	"offer.accepted":     "Offer accepted",
	"offer.rejected":     "Offer rejected",
	"contract.created":   "Contract created",
	"contract.updated":   "Contract edited",
	"contract.archived":  "Contract archived",
	"project.archived":   "Project archived",
}

func activityLine(item chatlinks.Item, shared sharedText) string {
	subject := strings.TrimSpace(shared.subject)
	if subject == "" {
		subject = "(no subject)"
	}
	if item.EventType == "activity.updated" {
		if fields, ok := item.Detail["fields"].(map[string]any); ok && fields["meeting_status"] == "held" {
			return "Meeting held: " + subject
		}
		return "Done: " + subject
	}
	return fmt.Sprintf("New %s: %s", shared.kind, subject)
}

var chatFieldWords = map[string]string{
	"amount_minor":        "amount",
	"currency":            "currency",
	"expected_close_date": "close date",
	"name":                "name",
	"owner_id":            "owner",
	"target_end_date":     "target end",
}

func changedFieldWords(detail map[string]any) string {
	fields, _ := detail["fields"].(map[string]any)
	var words []string
	for field := range fields {
		word, ok := chatFieldWords[field]
		if !ok {
			word = strings.ReplaceAll(field, "_", " ")
		}
		words = append(words, word)
	}
	if len(words) == 0 {
		return "details"
	}
	slices.Sort(words)
	return strings.Join(words, ", ")
}
