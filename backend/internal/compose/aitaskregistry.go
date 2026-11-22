// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Compose binds the task contract to the concrete invocation sites this build serves.

import (
	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/cardphoto"
	"github.com/margince/margince/backend/internal/compose/chatsummary"
	"github.com/margince/margince/backend/internal/compose/chattasks"
	"github.com/margince/margince/backend/internal/compose/goalproposal"
	"github.com/margince/margince/backend/internal/compose/reportsetup"
	"github.com/margince/margince/backend/internal/compose/rewrite"
	"github.com/margince/margince/backend/internal/compose/searchplan"
	"github.com/margince/margince/backend/internal/modules/ai"
)

// NewTaskCensus registers concrete sites and their certification cases.
// Validate checks those independent claims against the task contract.
// Deriving this list from ai.SitesFor would compare the contract with itself.
func NewTaskCensus() (*aitasks.Registry, error) {
	r := aitasks.NewRegistry()
	register := func(kind string, task ai.Task, variant string, c aitasks.CaseFactory) {
		site := aitasks.Site{Task: task, Variant: variant, Kind: kind}
		r.Register(site)
		r.BindCase(site, c)
	}
	oneShot := func(task ai.Task, variant string, c aitasks.CaseFactory) {
		register(ai.SiteKindOneShot, task, variant, c)
	}
	multiTurn := func(task ai.Task, variant string, c aitasks.CaseFactory) {
		register(ai.SiteKindMultiTurn, task, variant, c)
	}

	oneShot(ai.TaskCaptureClassify, "classify", captureClassifyCases{})
	oneShot(ai.TaskCaptureClassify, "deal_topic", dealTopicCases{})
	oneShot(ai.TaskOwedVerdict, "owed", owedVerdictCases{})
	oneShot(ai.TaskRequestSettlement, "request_settle", requestSettleCases{})
	oneShot(ai.TaskDealVerdict, "deal_verdict", dealVerdictCases{})
	oneShot(ai.TaskTagSuggest, "interest", tagSuggestCases{})
	oneShot(ai.TaskCaptureCounterpartyVerdict, "verdict", counterpartyVerdictCases{})
	oneShot(ai.TaskCaptureConfidentialityVerdict, "thread", confidentialityCases{})
	oneShot(ai.TaskEnrich, "signature", signatureEnrichCases{})
	oneShot(ai.TaskDraftReply, "reply", replyDraftCases{})
	oneShot(ai.TaskDraftReply, "contact", contactDraftCases{})
	oneShot(ai.TaskDraftReply, "account", accountDraftCases{})
	oneShot(ai.TaskSummarize, "meeting_plan", meetingPlanCases{})
	oneShot(ai.TaskSummarize, "meeting_brief", meetingBriefCases{})
	oneShot(ai.TaskDraftReply, "intro", introDraftCases{})
	oneShot(ai.TaskDraftReply, "intro_note", introNoteCases{})
	oneShot(ai.TaskRewrite, "rewrite", rewrite.CertCases{})
	oneShot(ai.TaskChatSummary, "episode", chatsummary.CertCases{})
	oneShot(ai.TaskChatTaskReading, "chat_tasks", chattasks.CertCases{})
	oneShot(ai.TaskProposeRoles, "committee", proposeRolesCases{})
	oneShot(ai.TaskNlSearch, "filter_propose", filterProposeCases{})
	oneShot(ai.TaskNlSearch, "search_data_plan", searchplan.Cases{Vocabulary: searchCaseVocabulary})
	oneShot(ai.TaskReaderRendering, "activity_text", readerRenderingCases{})
	oneShot(ai.TaskReportInterpret, "sales_assessment", salesReviewCases)
	oneShot(ai.TaskReportInterpret, "progress_summary", progressSummaryCases)
	oneShot(ai.TaskReportSetup, "initiative_setup", reportSetupCases{site: reportsetup.SiteInitiative})
	oneShot(ai.TaskReportSetup, "custom_report_setup", reportSetupCases{site: reportsetup.SiteCustomReport})
	oneShot(ai.TaskReportSetup, "sales_goal_months", goalproposal.Cases{})
	oneShot(ai.TaskBriefRanking, "rank", briefRankingCases{})
	oneShot(ai.TaskWeeklyReview, "narrative", weeklyNarrativeCases{})
	oneShot(ai.TaskWeeklyLearnings, "learn", weeklyLearningsCases{})
	oneShot(ai.TaskOfferDraft, "draft", offerDraftCases{})
	oneShot(ai.TaskSignalExtract, "thread_events", signalExtractCases{})
	oneShot(ai.TaskStageEvidenceExtract, "criteria", stageEvidenceCases{})
	oneShot(ai.TaskSiteExtract, "profile", siteProfileCases{})
	oneShot(ai.TaskSiteFactExtract, "page_facts", sitePageFactsCases{})
	oneShot(ai.TaskSiteTriage, "triage", siteTriageCases{})
	oneShot(ai.TaskDealHealth, "deal_status", dealStatusCases{})
	oneShot(ai.TaskAccountScan, "company_scan", companyScanCases{})
	oneShot(ai.TaskCorpusAsk, "corpus_ask", corpusAskCases{})
	oneShot(ai.TaskSummarize, "company_brief", companyBriefCases{})
	oneShot(ai.TaskSummarize, "company_ask", companyAskCases{})
	oneShot(ai.TaskSummarize, "company_dossier", companyDossierCases{})
	oneShot(ai.TaskSummarize, "contact_brief", contactBriefCases{})
	oneShot(ai.TaskSummarize, "deal_room_brief", roomBriefCases{})
	oneShot(ai.TaskTranscriptPropose, "next_steps", transcriptProposeCases{})
	oneShot(ai.TaskDocumentExtract, "fields", documentFieldsCases{})
	oneShot(ai.TaskDocumentExtract, "cards", cardphoto.CertCases{})
	oneShot(ai.TaskDocumentExtract, laneChat, chatCaptureCases{})
	oneShot(ai.TaskGrowthFit, "growth_fit", growthFitCases{})
	oneShot(ai.TaskCertJudge, "judge", certJudgeCases{})
	oneShot(ai.TaskExtensionTask, "run", extensionTaskCases{})
	oneShot(ai.TaskRateExtract, "fx", rateFxCases{})
	oneShot(ai.TaskVoiceBuild, "derive", voiceDeriveCases{})
	oneShot(ai.TaskVoiceBuild, "eval_draft", voiceEvalDraftCases{})
	oneShot(ai.TaskVoiceBuild, "eval_scores", voiceEvalScoresCases{})
	oneShot(ai.TaskVoiceBuild, "demo_draft", voiceDemoDraftCases{})
	oneShot(ai.TaskColdStart, "field_extract", fieldExtractCases{})
	multiTurn(ai.TaskColdStart, "company_message", onboardingCompanyMessageCases{})
	multiTurn(ai.TaskColdStart, "sitereadmessage", companyReadMessageCases{})
	multiTurn(ai.TaskColdStart, "acts", onboardingActCases{})
	for _, agent := range ai.AgentsFor(ai.TaskAgentLoop) {
		register(ai.SiteKindAgentLoop, ai.TaskAgentLoop, agent.Name, agentLoopCases{agent: agent.Name})
	}

	return r, r.Validate()
}
