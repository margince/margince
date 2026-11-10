// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

func genericWriteMetadata(head *systemtext.Component, fields json.RawMessage) *systemtext.Message {
	var patch map[string]json.RawMessage
	if json.Unmarshal(fields, &patch) != nil || len(patch) == 0 {
		return &systemtext.Message{Subject: head}
	}
	names := make([]string, 0, len(patch))
	for name := range patch {
		names = append(names, name)
	}
	sort.Strings(names)
	fieldList := systemtext.Sentence("core.source", systemtext.Text(strings.Join(names, ", ")))
	if len(names) > summaryFieldLimit {
		fieldList = systemtext.Sentence("agent.fieldListMore", systemtext.Text(strings.Join(names[:summaryFieldLimit], ", ")), systemtext.Nested(systemtext.Sentence("agent.moreFields", systemtext.Count(int64(len(names)-summaryFieldLimit)))))
	}
	sentence := systemtext.Sequence(head, systemtext.Sentence("agent.settingFields", systemtext.Nested(fieldList)))
	sentence.Layout = systemtext.LayoutConcat
	return &systemtext.Message{Subject: sentence}
}

func accountSendComponent(cmd SendCompanyEmailCommand, links []RecordLink) *systemtext.Component {
	parts := []*systemtext.Component{}
	if len(cmd.Cc) > 0 {
		parts = append(parts, systemtext.Sentence("agent.cc", systemtext.Text(strings.Join(cmd.Cc, ", "))))
	}
	parts = append(parts, systemtext.Sentence("agent.accountSendFiled", systemtext.Text(cmd.Subject), systemtext.Count(int64(len(links)))))
	sentence := systemtext.Sequence(systemtext.Sentence("agent.accountSend", systemtext.Text(strings.Join(cmd.To, ", "))), parts...)
	sentence.Layout = systemtext.LayoutConcat
	return sentence
}

func bookingComponent(cmd BookMeetingCommand, links []RecordLink) *systemtext.Component {
	subject := systemtext.Text(cmd.Subject)
	if strings.TrimSpace(cmd.Subject) == "" {
		subject = systemtext.MessageArgument("agent.noSubject")
	}
	sentence := systemtext.Sentence("agent.booking", subject, systemtext.Date(cmd.Start.Format(time.RFC3339)), systemtext.Date(cmd.End.Format(time.RFC3339)))
	var following []*systemtext.Component
	if cmd.HostUserID != nil {
		following = append(following, systemtext.Sentence("agent.bookingHost", systemtext.Text(cmd.HostUserID.String())))
	}
	if len(links) > 0 {
		following = append(following, systemtext.Sentence("agent.bookingLinks", systemtext.Count(int64(len(links)))))
	}
	sentence = systemtext.Sequence(sentence, following...)
	sentence.Layout = systemtext.LayoutConcat
	return sentence
}

func annotationComponent(cmd AnnotateBriefCommand) *systemtext.Component {
	switch {
	case cmd.Items == 0 && !cmd.Narrative:
		return systemtext.Sentence("agent.annotateNothing")
	case cmd.Items == 0:
		return systemtext.Sentence("agent.annotateNarrative")
	case cmd.Narrative:
		return systemtext.Sentence("agent.annotateBoth", systemtext.Count(int64(cmd.Items)))
	default:
		return systemtext.Sentence("agent.annotateFindings", systemtext.Count(int64(cmd.Items)))
	}
}

func importComponent(object string, report crmcontracts.ImportRunReport) *systemtext.Component {
	d := report.Disposition
	first := systemtext.Sentence("agent.importCreate", systemtext.Count(int64(d.Created)))
	var counts []*systemtext.Component
	for _, part := range []struct {
		code  string
		count int
	}{{"agent.importUpdate", d.Updated}, {"agent.importUnchanged", d.Unchanged}, {"agent.importSkip", d.Skipped}} {
		if part.count > 0 {
			counts = append(counts, systemtext.Sentence(part.code, systemtext.Count(int64(part.count))))
		}
	}
	disposition := systemtext.Sequence(first, counts...)
	disposition.Layout = "commas"
	sentence := systemtext.Sentence("agent.importCommit", systemtext.Count(int64(report.RowsRead)), systemtext.NounArgument(object), systemtext.Nested(disposition))
	if len(report.Issues) > 0 {
		sentence = systemtext.Sequence(sentence, systemtext.Sentence("agent.importIssues", systemtext.Count(int64(len(report.Issues)))))
		sentence.Layout = systemtext.LayoutConcat
	}
	return sentence
}

func sendComponent(cmd SendEmailCommand) *systemtext.Component {
	var following []*systemtext.Component
	if len(cmd.Cc) > 0 {
		following = append(following, systemtext.Sentence("agent.cc", systemtext.Text(strings.Join(cmd.Cc, ", "))))
	}
	following = append(following, systemtext.Sentence("agent.sendSubject", systemtext.Text(cmd.Subject)))
	sentence := systemtext.Sequence(systemtext.Sentence("agent.sendEmail", systemtext.Text(strings.Join(cmd.To, ", "))), following...)
	sentence.Layout = systemtext.LayoutConcat
	return sentence
}

// describeAnnotation says what the pass wrote, in counts.
func describeAnnotation(said summaryCopy, cmd AnnotateBriefCommand) string {
	return systemtext.Canonical(annotationComponent(cmd), said.lang)
}
