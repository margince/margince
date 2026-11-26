// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package exttest

// The refusing ports Runtime hands out. System reads answer what the core
// answers off a job tick or bus delivery. Declared capabilities answer what it
// answers to a unit that did not declare them.

import (
	"context"

	"github.com/margince/margince/backend/pkg/extension"
)

type deals struct{}

func (deals) Get(context.Context, string) (extension.DealSummary, error) {
	return extension.DealSummary{}, extension.ErrAttendedRead
}

func (deals) Responsible(context.Context, string) ([]extension.Responsible, error) {
	return nil, extension.ErrAttendedRead
}

type authority struct{}

func (authority) CanRead(context.Context, extension.UserID, extension.EntityRef, []string) (bool, error) {
	return false, extension.ErrAttendedRead
}

func (authority) CanWrite(context.Context, extension.UserID, extension.EntityRef) (bool, error) {
	return false, extension.ErrAttendedRead
}

type directory struct{}

func (directory) Emails(context.Context, []extension.UserID) (map[extension.UserID]string, error) {
	return nil, extension.ErrAttendedRead
}

func (directory) ByRole(context.Context, extension.UserID, []string) ([]extension.UserID, error) {
	return nil, extension.ErrAttendedRead
}

type activities struct{}

func (activities) Mail(context.Context, extension.UserID, string, extension.MailQuery) (extension.Page[extension.MailSummary], error) {
	return extension.Page[extension.MailSummary]{}, extension.ErrAttendedRead
}

func (activities) Attachments(context.Context, extension.UserID, string) ([]extension.MailAttachment, error) {
	return nil, extension.ErrAttendedRead
}

func (activities) OpenAttachment(context.Context, extension.UserID, string) (extension.MailAttachmentFile, error) {
	return extension.MailAttachmentFile{}, extension.ErrAttendedRead
}

type setAside struct{}

func (setAside) Contact(context.Context, extension.UserID, string) (extension.SetAsideState, error) {
	return extension.SetAsideState{}, extension.ErrAttendedRead
}

func (setAside) Contacts(context.Context, extension.UserID, []string) (map[string]extension.SetAsideState, error) {
	return nil, extension.ErrAttendedRead
}

type companies struct{}

func (companies) Get(context.Context, extension.UserID, string) (extension.CompanySummary, error) {
	return extension.CompanySummary{}, extension.ErrAttendedRead
}

func (companies) List(context.Context, extension.UserID, extension.CompanyPageQuery) (extension.Page[extension.CompanySummary], error) {
	return extension.Page[extension.CompanySummary]{}, extension.ErrAttendedRead
}

type mailContents struct{}

func (mailContents) Get(context.Context, extension.UserID, string) (extension.MailContent, error) {
	return extension.MailContent{}, extension.ErrAttendedRead
}

type models struct{}

func (models) Complete(context.Context, string, extension.CompleteRequest) (string, error) {
	return "", extension.ErrNotDeclared
}

type objects struct{}

func (objects) Put(context.Context, string, []byte) error { return extension.ErrNotDeclared }
func (objects) Get(context.Context, string) (extension.Object, error) {
	return extension.Object{}, extension.ErrNotDeclared
}
func (objects) Delete(context.Context, string) error { return extension.ErrNotDeclared }

type knowledgeStub struct{}

func (knowledgeStub) UpsertDocument(context.Context, extension.KnowledgeDocument) (extension.KnowledgeResult, error) {
	return extension.KnowledgeResult{}, extension.ErrNotDeclared
}
func (knowledgeStub) DeleteDocument(context.Context, string) error { return extension.ErrNotDeclared }
func (knowledgeStub) ExternalIDs(context.Context) ([]string, error) {
	return nil, extension.ErrNotDeclared
}
