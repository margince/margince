// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The inbox lines a governed call writes when it stages for a human.
//
// An approval's summary is shared-record text: stored once and read by every
// seat, so it follows the installation's base language, resolved when the call
// is described. The same line reaches the calling agent in StageInfo, and the
// base language is right for that reader too. A record type reads as the
// language's noun; field names, addresses, subjects and labels are values, and
// pass through untranslated.

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/baselanguage"
)

// summaryCopy is one language's set. Every field is required, and the census
// (summarycopy_test.go) holds each one to the English sentence's formatting
// verbs, in the English order, because Go's verbs are positional.
type summaryCopy struct {
	// Record writes. A record type reads as the language's noun (recordnouns.go);
	// labels, field names and ids pass through.
	archive, createHead, updateHead, logHead, settingFields, moreFields, merge,
	mergeCase, previewCase, enrich, ownDomain string

	// Writes nested under a parent record, named by id.
	onDeal, applyTag, addLineItem, updateLineItem, removeLineItem,
	setStakeholder, removeStakeholder, setCompany, removeCompany,
	confirmFact, updateFact, createFact, deleteFact, confirmProfileField, updateProfileField,
	mergeTags, mergeMembers, changeList, dismissDuplicate, reopenDuplicate, setUpPartner string

	// Lifecycle moves. A deal's target and source are stage semantics and pass
	// through.
	promoteLead, disqualifyLead, demoteLead, projectPhase, dealMove, dealMoveOpen, dealReopen,
	dealChange, dealClose string

	// Correspondence. Addresses, subjects and bodies pass through, quoted where
	// the sentence quotes them.
	sendEmail, cc, sendSubject, sendMessage, draftReply, accountSend, accountSendFiled, booking,
	bookingHost, bookingLinks, hostNamed, noSubject, relinkActivity, relinkActivities string

	// Imports. The counts are the report's; the record type reads as a noun.
	importPreview, importCommit, importCreate, importUpdate, importUnchanged, importSkip,
	importIssues string

	// The agent's own work: decisions, reports, the morning brief and a volume
	// step-up.
	approveWord, rejectWord, decideApproval, decideBundle, runReport, composeReport, draftReport, bulkChange,
	analyticsQuery, annotateNothing, annotateNarrative, annotateBoth, annotateFindings,
	stepUpChanges string

	// lang is the language this set is written in, which names its record
	// types (recordnouns.go). It is not a sentence, so the census skips it.
	lang textlang.Lang
}

// summaryByLang is the census, keyed by textlang.Lang so the test can walk
// textlang.Shipped and ask this map directly.
//
//nolint:dupl // one block per language with the same keys is the table's shape; the census holds them in step
var summaryByLang = map[textlang.Lang]summaryCopy{
	textlang.English: {
		lang:                textlang.English,
		archive:             systemtext.Copy("agent.archive", textlang.English),
		createHead:          systemtext.Copy("agent.createHead", textlang.English),
		updateHead:          systemtext.Copy("agent.updateHead", textlang.English),
		logHead:             systemtext.Copy("agent.logHead", textlang.English),
		settingFields:       systemtext.Copy("agent.settingFields", textlang.English),
		moreFields:          systemtext.Copy("agent.moreFields", textlang.English),
		merge:               systemtext.Copy("agent.merge", textlang.English),
		mergeCase:           systemtext.Copy("agent.mergeCase", textlang.English),
		previewCase:         systemtext.Copy("agent.previewCase", textlang.English),
		enrich:              systemtext.Copy("agent.enrich", textlang.English),
		ownDomain:           systemtext.Copy("agent.ownDomain", textlang.English),
		onDeal:              systemtext.Copy("agent.onDeal", textlang.English),
		applyTag:            systemtext.Copy("agent.applyTag", textlang.English),
		dismissDuplicate:    systemtext.Copy("agent.dismissDuplicate", textlang.English),
		reopenDuplicate:     systemtext.Copy("agent.reopenDuplicate", textlang.English),
		changeList:          systemtext.Copy("agent.changeList", textlang.English),
		addLineItem:         systemtext.Copy("agent.addLineItem", textlang.English),
		updateLineItem:      systemtext.Copy("agent.updateLineItem", textlang.English),
		removeLineItem:      systemtext.Copy("agent.removeLineItem", textlang.English),
		setStakeholder:      systemtext.Copy("agent.setStakeholder", textlang.English),
		removeStakeholder:   systemtext.Copy("agent.removeStakeholder", textlang.English),
		setCompany:          systemtext.Copy("agent.setCompany", textlang.English),
		removeCompany:       systemtext.Copy("agent.removeCompany", textlang.English),
		confirmFact:         systemtext.Copy("agent.confirmFact", textlang.English),
		updateFact:          systemtext.Copy("agent.updateFact", textlang.English),
		createFact:          systemtext.Copy("agent.createFact", textlang.English),
		deleteFact:          systemtext.Copy("agent.deleteFact", textlang.English),
		confirmProfileField: systemtext.Copy("agent.confirmProfileField", textlang.English),
		updateProfileField:  systemtext.Copy("agent.updateProfileField", textlang.English),
		mergeTags:           systemtext.Copy("agent.mergeTags", textlang.English),
		mergeMembers:        systemtext.Copy("agent.mergeMembers", textlang.English),
		setUpPartner:        systemtext.Copy("agent.setUpPartner", textlang.English),
		promoteLead:         systemtext.Copy("agent.promoteLead", textlang.English),
		disqualifyLead:      systemtext.Copy("agent.disqualifyLead", textlang.English),
		demoteLead:          systemtext.Copy("agent.demoteLead", textlang.English),
		projectPhase:        systemtext.Copy("agent.projectPhase", textlang.English),
		dealMove:            systemtext.Copy("agent.dealMove", textlang.English),
		dealMoveOpen:        systemtext.Copy("agent.dealMoveOpen", textlang.English),
		dealReopen:          systemtext.Copy("agent.dealReopen", textlang.English),
		dealChange:          systemtext.Copy("agent.dealChange", textlang.English),
		dealClose:           systemtext.Copy("agent.dealClose", textlang.English),
		sendEmail:           systemtext.Copy("agent.sendEmail", textlang.English),
		cc:                  systemtext.Copy("agent.cc", textlang.English),
		sendSubject:         systemtext.Copy("agent.sendSubject", textlang.English),
		sendMessage:         systemtext.Copy("agent.sendMessage", textlang.English),
		draftReply:          systemtext.Copy("agent.draftReply", textlang.English),
		accountSend:         systemtext.Copy("agent.accountSend", textlang.English),
		accountSendFiled:    systemtext.Copy("agent.accountSendFiled", textlang.English),
		booking:             systemtext.Copy("agent.booking", textlang.English),
		bookingHost:         systemtext.Copy("agent.bookingHost", textlang.English),
		bookingLinks:        systemtext.Copy("agent.bookingLinks", textlang.English),
		hostNamed:           systemtext.Copy("agent.hostNamed", textlang.English),
		noSubject:           systemtext.Copy("agent.noSubject", textlang.English),
		relinkActivity:      systemtext.Copy("agent.relinkActivity", textlang.English),
		relinkActivities:    systemtext.Copy("agent.relinkActivities", textlang.English),
		importPreview:       systemtext.Copy("agent.importPreview", textlang.English),
		importCommit:        systemtext.Copy("agent.importCommit", textlang.English),
		importCreate:        systemtext.Copy("agent.importCreate", textlang.English),
		importUpdate:        systemtext.Copy("agent.importUpdate", textlang.English),
		importUnchanged:     systemtext.Copy("agent.importUnchanged", textlang.English),
		importSkip:          systemtext.Copy("agent.importSkip", textlang.English),
		importIssues:        systemtext.Copy("agent.importIssues", textlang.English),
		approveWord:         systemtext.Copy("agent.approveWord", textlang.English),
		rejectWord:          systemtext.Copy("agent.rejectWord", textlang.English),
		decideApproval:      systemtext.Copy("agent.decideApproval", textlang.English),
		decideBundle:        systemtext.Copy("agent.decideBundle", textlang.English),
		runReport:           systemtext.Copy("agent.runReport", textlang.English),
		composeReport:       systemtext.Copy("agent.composeReport", textlang.English),
		draftReport:         systemtext.Copy("agent.draftReport", textlang.English),
		bulkChange:          systemtext.Copy("agent.bulkChange", textlang.English),
		analyticsQuery:      systemtext.Copy("agent.analyticsQuery", textlang.English),
		annotateNothing:     systemtext.Copy("agent.annotateNothing", textlang.English),
		annotateNarrative:   systemtext.Copy("agent.annotateNarrative", textlang.English),
		annotateBoth:        systemtext.Copy("agent.annotateBoth", textlang.English),
		annotateFindings:    systemtext.Copy("agent.annotateFindings", textlang.English),
		stepUpChanges:       systemtext.Copy("agent.stepUpChanges", textlang.English),
	},
	textlang.German: {
		lang:                textlang.German,
		archive:             systemtext.Copy("agent.archive", textlang.German),
		createHead:          systemtext.Copy("agent.createHead", textlang.German),
		updateHead:          systemtext.Copy("agent.updateHead", textlang.German),
		logHead:             systemtext.Copy("agent.logHead", textlang.German),
		settingFields:       systemtext.Copy("agent.settingFields", textlang.German),
		moreFields:          systemtext.Copy("agent.moreFields", textlang.German),
		merge:               systemtext.Copy("agent.merge", textlang.German),
		mergeCase:           systemtext.Copy("agent.mergeCase", textlang.German),
		previewCase:         systemtext.Copy("agent.previewCase", textlang.German),
		enrich:              systemtext.Copy("agent.enrich", textlang.German),
		ownDomain:           systemtext.Copy("agent.ownDomain", textlang.German),
		onDeal:              systemtext.Copy("agent.onDeal", textlang.German),
		applyTag:            systemtext.Copy("agent.applyTag", textlang.German),
		dismissDuplicate:    systemtext.Copy("agent.dismissDuplicate", textlang.German),
		reopenDuplicate:     systemtext.Copy("agent.reopenDuplicate", textlang.German),
		changeList:          systemtext.Copy("agent.changeList", textlang.German),
		addLineItem:         systemtext.Copy("agent.addLineItem", textlang.German),
		updateLineItem:      systemtext.Copy("agent.updateLineItem", textlang.German),
		removeLineItem:      systemtext.Copy("agent.removeLineItem", textlang.German),
		setStakeholder:      systemtext.Copy("agent.setStakeholder", textlang.German),
		removeStakeholder:   systemtext.Copy("agent.removeStakeholder", textlang.German),
		setCompany:          systemtext.Copy("agent.setCompany", textlang.German),
		removeCompany:       systemtext.Copy("agent.removeCompany", textlang.German),
		confirmFact:         systemtext.Copy("agent.confirmFact", textlang.German),
		updateFact:          systemtext.Copy("agent.updateFact", textlang.German),
		createFact:          systemtext.Copy("agent.createFact", textlang.German),
		deleteFact:          systemtext.Copy("agent.deleteFact", textlang.German),
		confirmProfileField: systemtext.Copy("agent.confirmProfileField", textlang.German),
		updateProfileField:  systemtext.Copy("agent.updateProfileField", textlang.German),
		mergeTags:           systemtext.Copy("agent.mergeTags", textlang.German),
		mergeMembers:        systemtext.Copy("agent.mergeMembers", textlang.German),
		setUpPartner:        systemtext.Copy("agent.setUpPartner", textlang.German),
		promoteLead:         systemtext.Copy("agent.promoteLead", textlang.German),
		disqualifyLead:      systemtext.Copy("agent.disqualifyLead", textlang.German),
		demoteLead:          systemtext.Copy("agent.demoteLead", textlang.German),
		projectPhase:        systemtext.Copy("agent.projectPhase", textlang.German),
		dealMove:            systemtext.Copy("agent.dealMove", textlang.German),
		dealMoveOpen:        systemtext.Copy("agent.dealMoveOpen", textlang.German),
		dealReopen:          systemtext.Copy("agent.dealReopen", textlang.German),
		dealChange:          systemtext.Copy("agent.dealChange", textlang.German),
		dealClose:           systemtext.Copy("agent.dealClose", textlang.German),
		sendEmail:           systemtext.Copy("agent.sendEmail", textlang.German),
		cc:                  systemtext.Copy("agent.cc", textlang.German),
		sendSubject:         systemtext.Copy("agent.sendSubject", textlang.German),
		sendMessage:         systemtext.Copy("agent.sendMessage", textlang.German),
		draftReply:          systemtext.Copy("agent.draftReply", textlang.German),
		accountSend:         systemtext.Copy("agent.accountSend", textlang.German),
		accountSendFiled:    systemtext.Copy("agent.accountSendFiled", textlang.German),
		booking:             systemtext.Copy("agent.booking", textlang.German),
		bookingHost:         systemtext.Copy("agent.bookingHost", textlang.German),
		bookingLinks:        systemtext.Copy("agent.bookingLinks", textlang.German),
		hostNamed:           systemtext.Copy("agent.hostNamed", textlang.German),
		noSubject:           systemtext.Copy("agent.noSubject", textlang.German),
		relinkActivity:      systemtext.Copy("agent.relinkActivity", textlang.German),
		relinkActivities:    systemtext.Copy("agent.relinkActivities", textlang.German),
		importPreview:       systemtext.Copy("agent.importPreview", textlang.German),
		importCommit:        systemtext.Copy("agent.importCommit", textlang.German),
		importCreate:        systemtext.Copy("agent.importCreate", textlang.German),
		importUpdate:        systemtext.Copy("agent.importUpdate", textlang.German),
		importUnchanged:     systemtext.Copy("agent.importUnchanged", textlang.German),
		importSkip:          systemtext.Copy("agent.importSkip", textlang.German),
		importIssues:        systemtext.Copy("agent.importIssues", textlang.German),
		approveWord:         systemtext.Copy("agent.approveWord", textlang.German),
		rejectWord:          systemtext.Copy("agent.rejectWord", textlang.German),
		decideApproval:      systemtext.Copy("agent.decideApproval", textlang.German),
		decideBundle:        systemtext.Copy("agent.decideBundle", textlang.German),
		runReport:           systemtext.Copy("agent.runReport", textlang.German),
		composeReport:       systemtext.Copy("agent.composeReport", textlang.German),
		draftReport:         systemtext.Copy("agent.draftReport", textlang.German),
		bulkChange:          systemtext.Copy("agent.bulkChange", textlang.German),
		analyticsQuery:      systemtext.Copy("agent.analyticsQuery", textlang.German),
		annotateNothing:     systemtext.Copy("agent.annotateNothing", textlang.German),
		annotateNarrative:   systemtext.Copy("agent.annotateNarrative", textlang.German),
		annotateBoth:        systemtext.Copy("agent.annotateBoth", textlang.German),
		annotateFindings:    systemtext.Copy("agent.annotateFindings", textlang.German),
		stepUpChanges:       systemtext.Copy("agent.stepUpChanges", textlang.German),
	},
	textlang.Vietnamese: {
		lang:                textlang.Vietnamese,
		archive:             systemtext.Copy("agent.archive", textlang.Vietnamese),
		createHead:          systemtext.Copy("agent.createHead", textlang.Vietnamese),
		updateHead:          systemtext.Copy("agent.updateHead", textlang.Vietnamese),
		logHead:             systemtext.Copy("agent.logHead", textlang.Vietnamese),
		settingFields:       systemtext.Copy("agent.settingFields", textlang.Vietnamese),
		moreFields:          systemtext.Copy("agent.moreFields", textlang.Vietnamese),
		merge:               systemtext.Copy("agent.merge", textlang.Vietnamese),
		mergeCase:           systemtext.Copy("agent.mergeCase", textlang.Vietnamese),
		previewCase:         systemtext.Copy("agent.previewCase", textlang.Vietnamese),
		enrich:              systemtext.Copy("agent.enrich", textlang.Vietnamese),
		ownDomain:           systemtext.Copy("agent.ownDomain", textlang.Vietnamese),
		onDeal:              systemtext.Copy("agent.onDeal", textlang.Vietnamese),
		applyTag:            systemtext.Copy("agent.applyTag", textlang.Vietnamese),
		dismissDuplicate:    systemtext.Copy("agent.dismissDuplicate", textlang.Vietnamese),
		reopenDuplicate:     systemtext.Copy("agent.reopenDuplicate", textlang.Vietnamese),
		changeList:          systemtext.Copy("agent.changeList", textlang.Vietnamese),
		addLineItem:         systemtext.Copy("agent.addLineItem", textlang.Vietnamese),
		updateLineItem:      systemtext.Copy("agent.updateLineItem", textlang.Vietnamese),
		removeLineItem:      systemtext.Copy("agent.removeLineItem", textlang.Vietnamese),
		setStakeholder:      systemtext.Copy("agent.setStakeholder", textlang.Vietnamese),
		removeStakeholder:   systemtext.Copy("agent.removeStakeholder", textlang.Vietnamese),
		setCompany:          systemtext.Copy("agent.setCompany", textlang.Vietnamese),
		removeCompany:       systemtext.Copy("agent.removeCompany", textlang.Vietnamese),
		confirmFact:         systemtext.Copy("agent.confirmFact", textlang.Vietnamese),
		updateFact:          systemtext.Copy("agent.updateFact", textlang.Vietnamese),
		createFact:          systemtext.Copy("agent.createFact", textlang.Vietnamese),
		deleteFact:          systemtext.Copy("agent.deleteFact", textlang.Vietnamese),
		confirmProfileField: systemtext.Copy("agent.confirmProfileField", textlang.Vietnamese),
		updateProfileField:  systemtext.Copy("agent.updateProfileField", textlang.Vietnamese),
		mergeTags:           systemtext.Copy("agent.mergeTags", textlang.Vietnamese),
		mergeMembers:        systemtext.Copy("agent.mergeMembers", textlang.Vietnamese),
		setUpPartner:        systemtext.Copy("agent.setUpPartner", textlang.Vietnamese),
		promoteLead:         systemtext.Copy("agent.promoteLead", textlang.Vietnamese),
		disqualifyLead:      systemtext.Copy("agent.disqualifyLead", textlang.Vietnamese),
		demoteLead:          systemtext.Copy("agent.demoteLead", textlang.Vietnamese),
		projectPhase:        systemtext.Copy("agent.projectPhase", textlang.Vietnamese),
		dealMove:            systemtext.Copy("agent.dealMove", textlang.Vietnamese),
		dealMoveOpen:        systemtext.Copy("agent.dealMoveOpen", textlang.Vietnamese),
		dealReopen:          systemtext.Copy("agent.dealReopen", textlang.Vietnamese),
		dealChange:          systemtext.Copy("agent.dealChange", textlang.Vietnamese),
		dealClose:           systemtext.Copy("agent.dealClose", textlang.Vietnamese),
		sendEmail:           systemtext.Copy("agent.sendEmail", textlang.Vietnamese),
		cc:                  systemtext.Copy("agent.cc", textlang.Vietnamese),
		sendSubject:         systemtext.Copy("agent.sendSubject", textlang.Vietnamese),
		sendMessage:         systemtext.Copy("agent.sendMessage", textlang.Vietnamese),
		draftReply:          systemtext.Copy("agent.draftReply", textlang.Vietnamese),
		accountSend:         systemtext.Copy("agent.accountSend", textlang.Vietnamese),
		accountSendFiled:    systemtext.Copy("agent.accountSendFiled", textlang.Vietnamese),
		booking:             systemtext.Copy("agent.booking", textlang.Vietnamese),
		bookingHost:         systemtext.Copy("agent.bookingHost", textlang.Vietnamese),
		bookingLinks:        systemtext.Copy("agent.bookingLinks", textlang.Vietnamese),
		hostNamed:           systemtext.Copy("agent.hostNamed", textlang.Vietnamese),
		noSubject:           systemtext.Copy("agent.noSubject", textlang.Vietnamese),
		relinkActivity:      systemtext.Copy("agent.relinkActivity", textlang.Vietnamese),
		relinkActivities:    systemtext.Copy("agent.relinkActivities", textlang.Vietnamese),
		importPreview:       systemtext.Copy("agent.importPreview", textlang.Vietnamese),
		importCommit:        systemtext.Copy("agent.importCommit", textlang.Vietnamese),
		importCreate:        systemtext.Copy("agent.importCreate", textlang.Vietnamese),
		importUpdate:        systemtext.Copy("agent.importUpdate", textlang.Vietnamese),
		importUnchanged:     systemtext.Copy("agent.importUnchanged", textlang.Vietnamese),
		importSkip:          systemtext.Copy("agent.importSkip", textlang.Vietnamese),
		importIssues:        systemtext.Copy("agent.importIssues", textlang.Vietnamese),
		approveWord:         systemtext.Copy("agent.approveWord", textlang.Vietnamese),
		rejectWord:          systemtext.Copy("agent.rejectWord", textlang.Vietnamese),
		decideApproval:      systemtext.Copy("agent.decideApproval", textlang.Vietnamese),
		decideBundle:        systemtext.Copy("agent.decideBundle", textlang.Vietnamese),
		runReport:           systemtext.Copy("agent.runReport", textlang.Vietnamese),
		composeReport:       systemtext.Copy("agent.composeReport", textlang.Vietnamese),
		draftReport:         systemtext.Copy("agent.draftReport", textlang.Vietnamese),
		bulkChange:          systemtext.Copy("agent.bulkChange", textlang.Vietnamese),
		analyticsQuery:      systemtext.Copy("agent.analyticsQuery", textlang.Vietnamese),
		annotateNothing:     systemtext.Copy("agent.annotateNothing", textlang.Vietnamese),
		annotateNarrative:   systemtext.Copy("agent.annotateNarrative", textlang.Vietnamese),
		annotateBoth:        systemtext.Copy("agent.annotateBoth", textlang.Vietnamese),
		annotateFindings:    systemtext.Copy("agent.annotateFindings", textlang.Vietnamese),
		stepUpChanges:       systemtext.Copy("agent.stepUpChanges", textlang.Vietnamese),
	},
}

// summaryIn answers the set for the installation's base language, and English
// for a language this table has not learned: the language comes off a settings
// row, and a sentence in English beats no sentence.
func summaryIn(ctx context.Context, language baselanguage.Resolver) summaryCopy {
	return summaryFor(language.Resolve(ctx))
}

// summaryFor answers the set for lang, and the English set for a language the
// table has not learned.
func summaryFor(lang textlang.Lang) summaryCopy {
	if said, ok := summaryByLang[lang]; ok {
		return said
	}
	return summaryByLang[textlang.English]
}

// WithBaseLanguage injects the installation's base language every staged
// summary on this surface is written in. A registry composed without one
// writes English.
func WithBaseLanguage(language baselanguage.Resolver) RegistryOption {
	return func(r *Registry) { r.language = language }
}

// BaseLanguage answers the resolver this surface was composed with, so the
// REST door describes a call in the same language the tool door does.
func (r *Registry) BaseLanguage() baselanguage.Resolver {
	return r.language
}
