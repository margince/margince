<!-- prose:plain -->
# Agent tools

An agent in Margince acts for the human who gave it a **passport**, the credential an agent carries
([how to make one](../how-to/mint-a-passport.md)). It can only do what that human could do alone.

Most tools run at once for an agent that acts as its own user, because it can do only what that user could do alone. Some
still wait for a human then. These are the `enrich` tool and these actions: `set_up_partner` and `merge_member_accounts` on
`change_state`, and `relink` and `relink_many` on `change_activities`.

Automatic work with no passport behind it waits on every 🟡 tool in the catalog below, such as `merge` on `change_tags` and
`invite` on `schedule_meeting`. A few more wait only in some cases, such as moving a deal to `won` or `lost`. These are the
`update_record` tool and these actions: `move_deal` on `change_state`, and `relink` and `relink_many` on `change_activities`.

A waiting call changes nothing until a human approves it in the CRM. The agent then repeats the call with the approval's ID.

## Which page do you need?

| Your question | Page |
|---|---|
| What can an agent call, and does a human have to approve it? | This page |
| What does each scheduled agent's tool menu cost the context window? | [agent-tool-budget.md](agent-tool-budget.md) |
| What does a connecting client get, word for word? | [mcp-info.md](mcp-info.md) |
| How do the transport, sessions, limits and client sign-in work? | [mcp-protocol.md](mcp-protocol.md) |
| Can a given assistant do the everyday jobs? | [mcp-tool-coverage.md](mcp-tool-coverage.md) |
| Which role can use which tool? | [mcp-tool-roles.md](mcp-tool-roles.md) |

The reasons behind the rules are in [authorization.md](../explanation/authorization.md) and
[agent-surface.md](../explanation/agent-surface.md).

## How to read the tables

**Does a human have to approve it?** Only for rows marked 🟡 or dynamic, and for the 🟡 part of `update_record`. Every
other tool runs at once.

| Column | Meaning |
|---|---|
| **Tier** | 🟢 runs at once. 🟡 does nothing until a human approves it. **dynamic** is decided on each call: a deal moved to `won` waits, a move to an open stage runs. |
| **Scope** | The kind of action the passport must allow: `read`, `draft`, `write`, `send` or `enrich`. [What each scope buys](#what-each-scope-buys) says more. |
| **Egress** | Whether the tool leaves Margince, for example to send an email. |

## The catalog

### Tools a client lists

Sorted by scope, so the read-only tools come first. A 🔀 marks a tool that fronts several actions. An argument says which action to run, and each runs as its own verb, at that verb's tier. All the actions of one tool need the same scope. A tool whose actions differ in tier is listed as **dynamic**.

Open the row to see each action, the verb it runs and its tier, for example `merge` under `change_tags`. Approvals, tasks and the hidden-verb log name the verb. [agent-surface.md](../explanation/agent-surface.md) says how this works.

| Tool | Tier | Scope | Egress | For |
|---|---|---|---|---|
| `check_availability` | 🟢 | `read` | no | Check free times on a calendar |
| `find_records` | 🟢 | `read` | no | 🔀 Find records by text, filters, a query, meaning or a name <details><summary>actions</summary>`identify` (`resolve_entities`, 🟢) `list` (`list_records`, 🟢) `meaning` (`search_context`, 🟢) `query` (`query_workspace`, 🟢) `search` (`search_records`, 🟢)</details> |
| `list_approvals` | 🟢 | `read` | no | List what is waiting for a decision |
| `list_documents` | 🟢 | `read` | no | List a record's documents or read one <details><summary>actions</summary>`download` (`download_attachment`, 🟢) `list` (`list_record_documents`, 🟢)</details> |
| `read_analytics` | 🟢 | `read` | no | 🔀 Ask a question of the data, or build a report from it <details><summary>actions</summary>`compose` (`compose_analytics_report`, 🟢) `query` (`run_analytics_query`, 🟢)</details> |
| `read_approval` | 🟢 | `read` | no | Read one staged action in full |
| `read_attention` | 🟢 | `read` | no | 🔀 See the deals, relationships and promises that need you <details><summary>actions</summary>`at_risk` (`at_risk_relationships`, 🟢) `commitments` (`review_commitments`, 🟢) `slipping` (`whats_slipping_this_week`, 🟢)</details> |
| `read_context` | 🟢 | `read` | no | 🔀 Get the story so far on a record, a meeting or the morning <details><summary>actions</summary>`brief` (`read_brief`, 🟢) `catch_up` (`catch_me_up_on`, 🟢) `meeting` (`prep_for_meeting`, 🟢)</details> |
| `read_details` | 🟢 | `read` | no | 🔀 Read a record's panels and the day's lists <details><summary>actions</summary>`activity_pipeline` (`read_activity_pipeline_trace`, 🟢) `capture_triage` (`get_company_capture_triage`, 🟢) `company_brief` (`get_company_brief`, 🟢) `company_contracts` (`list_company_contracts`, 🟢) `company_for_address` (`get_company_for_address`, 🟢) `company_linkedin_reach` (`get_company_linked_in_reach`, 🟢) `company_logo_icon` (`get_company_logo_icon`, 🟢) `company_logo` (`get_company_logo`, 🟢) `company_scan` (`get_company_scan`, 🟢) `contact_brief` (`get_contact_brief`, 🟢) `contact_profile_fields` (`get_contact_profile_fields`, 🟢) `contract` (`get_contract`, 🟢) `deal_legal_reviews` (`list_deal_legal_reviews`, 🟢) `deal_products` (`list_deal_products`, 🟢) `deal_room_brief` (`get_deal_room_brief`, 🟢) `deal_suggestion` (`get_deal_suggestion`, 🟢) `deal_suggestions` (`list_deal_suggestions`, 🟢) `document_extraction` (`get_attachment_extraction`, 🟢) `dossier` (`get_company_dossier`, 🟢) `enrichment_run` (`get_contact_enrichment_run`, 🟢) `evidence` (`get_claim_evidence`, 🟢) `field_history` (`get_field_history`, 🟢) `forecast_assurance_enforcement` (`get_forecast_assurance_enforcement`, 🟢) `forecast_assurance_preview` (`preview_forecast_assurance`, 🟢) `forecast_share_csv` (`export_forecast_share`, 🟢) `forecast_shared_reading` (`open_forecast_share`, 🟢) `forecast_shares` (`list_forecast_shares`, 🟢) `forecast_stage_chances` (`get_forecast_stage_chances`, 🟢) `growth_fit` (`get_company_growth_fit`, 🟢) `handled_for_you` (`get_handled_for_you`, 🟢) `hidden_backlog_rows` (`get_hidden_backlog_rows`, 🟢) `hidden_backlog` (`get_hidden_backlog`, 🟢) `import_problems` (`download_import_problems`, 🟢) `import_template` (`download_import_template`, 🟢) `intro_requests` (`list_intro_requests`, 🟢) `knowledge_document` (`download_corpus_document`, 🟢) `latest_weekly_review` (`get_latest_weekly_review`, 🟢) `lead_promotion_preview` (`preview_lead_promotion`, 🟢) `lead_score` (`explain_lead_score`, 🟢) `lead_signals` (`list_lead_manual_signals`, 🟢) `legal_entity_logo` (`get_legal_entity_logo`, 🟢) `legal_review` (`get_legal_review`, 🟢) `legal_reviews` (`list_legal_reviews`, 🟢) `link_candidates` (`list_company_link_candidates`, 🟢) `magic_line_records` (`get_magic_line_records`, 🟢) `magic` (`get_magic`, 🟢) `mail_draft` (`get_mail_draft`, 🟢) `meeting_availability` (`get_meeting_availability`, 🟢) `meeting_proposals` (`list_meeting_proposals`, 🟢) `my_linkedin_reach` (`get_my_linked_in_reach`, 🟢) `notices` (`list_notices`, 🟢) `offer_pdf` (`download_offer_pdf`, 🟢) `offered_values` (`list_company_offered_values`, 🟢) `ownership_offer_candidates` (`list_ownership_offer_candidates`, 🟢) `ownership_offer` (`get_ownership_offer`, 🟢) `ownership_request` (`get_ownership_request`, 🟢) `project_filing` (`get_activity_project_filing`, 🟢) `relationship_nudge` (`get_relationship_nudge`, 🟢) `response_metrics` (`get_response_metrics`, 🟢) `scheduled_send` (`get_scheduled_send`, 🟢) `scheduled_sends` (`list_scheduled_sends`, 🟢) `scheduling_calendars` (`get_scheduling_calendars`, 🟢) `tag_suggestion` (`get_tag_suggestion`, 🟢) `team_board` (`get_team_board`, 🟢) `team_exceptions` (`get_team_exceptions`, 🟢) `team_weekly_review` (`get_team_weekly_review`, 🟢) `teammate_weekly_plan` (`get_teammate_weekly_plan`, 🟢) `weekly_plan` (`get_current_weekly_plan`, 🟢) `weekly_reviews` (`list_weekly_reviews`, 🟢) `win_evidence` (`get_deal_win_evidence`, 🟢)</details> |
| `read_forecast` | 🟢 | `read` | no | 🔀 Read the forecast, how it moved, and whether its inputs hold <details><summary>actions</summary>`assurance_enforcement` (`get_forecast_assurance_enforcement`, 🟢) `assurance_preview` (`preview_forecast_assurance`, 🟢) `input_exceptions` (`list_input_checks`, 🟢) `input_sources` (`data_coverage`, 🟢) `input_summary` (`forecast_input_checks`, 🟢) `movement` (`forecast_movement`, 🟢) `readings` (`forecast_readings`, 🟢) `shared_reading` (`open_forecast_share`, 🟢) `shares` (`list_forecast_shares`, 🟢) `stage_chances` (`get_forecast_stage_chances`, 🟢)</details> |
| `read_import` | 🟢 | `read` | no | 🔀 Read an import run or the import format <details><summary>actions</summary>`format` (`describe_import_format`, 🟢) `report` (`read_import_report`, 🟢)</details> |
| `read_lists` | 🟢 | `read` | no | Find and read lists |
| `read_project` | 🟢 | `read` | no | 🔀 Read a project, or what to pass on <details><summary>actions</summary>`handoff` (`prepare_handoff`, 🟢) `overview` (`read_project_360`, 🟢)</details> |
| `read_record` | 🟢 | `read` | no | Read a record |
| `read_relationships` | 🟢 | `read` | no | 🔀 Read who we know, and how well <details><summary>actions</summary>`company_id` (`intro_path_to`, 🟢) `contact_id` (`who_knows`, 🟢) `deal_id` (`company_coverage`, 🟢)</details> |
| `read_reports` | 🟢 | `read` | no | 🔀 Read the sales reports <details><summary>actions</summary>`analytics_context` (`get_analytics_context`, 🟢) `analytics_schema` (`get_analytics_schema`, 🟢) `evidence` (`search_report_evidence`, 🟢) `question_card_answer` (`answer_reporting_question_card`, 🟢) `reader_preview` (`preview_reporting_report_as_role`, 🟢) `report_run` (`get_report_run`, 🟢) `report_runs` (`list_report_runs`, 🟢) `run` (`run_report`, 🟢) `sales_decisions` (`list_sales_decisions`, 🟢) `sales_initiative_members` (`list_sales_initiative_members`, 🟢) `sales_initiative` (`get_sales_initiative`, 🟢) `sales_initiatives` (`list_sales_initiatives`, 🟢) `sdr_handoffs` (`list_sdr_waiting_handoffs`, 🟢) `sdr_meetings` (`list_sdr_upcoming_meetings`, 🟢) `sdr_summary` (`get_sdr_summary`, 🟢) `sdr_team` (`get_sdr_team_review`, 🟢) `standard` (`read_reporting`, 🟢)</details> |
| `read_tags` | 🟢 | `read` | no | 🔀 Read tags <details><summary>actions</summary>`get` (`get_tag`, 🟢) `list` (`list_tags`, 🟢) `record` (`get_record_tags`, 🟢)</details> |
| `read_view` | 🟢 | `read` | no | Read a view: a record's derived views, reference lists and reporting objects, each as its REST route answers |
| `read_workspace` | 🟢 | `read` | no | 🔀 Read how this workspace is set up and which names each tool takes <details><summary>actions</summary>`actions` (`describe_router_actions`, 🟢) `agent_tools` (`list_agent_tools`, 🟢) `ai_budget` (`get_ai_budget`, 🟢) `ai_call_stats` (`get_ai_call_stats`, 🟢) `ai_health` (`get_ai_health`, 🟢) `ai_profile` (`get_ai_profile`, 🟢) `ai_provider_health` (`get_ai_provider_health`, 🟢) `ai_routing_schema` (`get_ai_routing_schema`, 🟢) `ai_status` (`get_ai_status`, 🟢) `ai_task_flow` (`get_ai_task_flow`, 🟢) `ai_task_overrides` (`get_ai_task_overrides`, 🟢) `ai_usage_summary` (`get_ai_usage_summary`, 🟢) `ai_usage` (`get_ai_usage`, 🟢) `analytics` (`describe_analytics_vocabulary`, 🟢) `anchor_company_context_capabilities` (`get_anchor_company_context_capabilities`, 🟢) `anchor_company_context` (`get_anchor_company_context`, 🟢) `anchor_company_site_read` (`get_anchor_company_site_read`, 🟢) `anchor_company` (`get_anchor_company`, 🟢) `assignable_roles` (`list_assignable_roles`, 🟢) `automation_runs` (`list_automation_runs`, 🟢) `available_models` (`list_available_models`, 🟢) `blocked_domains` (`list_blocked_domains`, 🟢) `capture_exclusions` (`list_capture_exclusions`, 🟢) `capture_trace_pipeline` (`read_capture_trace_pipeline`, 🟢) `channel_connections` (`list_channel_connections`, 🟢) `channel_providers` (`list_channel_providers`, 🟢) `colleagues` (`list_colleagues`, 🟢) `confirm_submissions` (`list_confirm_submissions`, 🟢) `connector_containers` (`list_connector_containers`, 🟢) `connectors` (`list_connectors`, 🟢) `consumer_mail_baseline` (`list_consumer_mail_baseline`, 🟢) `consumer_mail_domains` (`list_consumer_mail_domains`, 🟢) `contact_case_settings` (`get_contact_case_settings`, 🟢) `controller_particulars` (`get_controller_particulars`, 🟢) `corpora` (`list_corpora`, 🟢) `corpus_documents` (`list_corpus_documents`, 🟢) `corpus` (`read_corpus`, 🟢) `domain_questions` (`list_domain_questions`, 🟢) `embed_reindex_identities` (`embed_reindex_identities`, 🟢) `embed_reindex_preview` (`embed_reindex_preview`, 🟢) `embed_reindex_status` (`embed_reindex_status`, 🟢) `extension_settings` (`get_extension_settings`, 🟢) `extensions` (`list_extensions`, 🟢) `finance_source_customers` (`list_finance_source_customers`, 🟢) `finance_source` (`get_finance_source`, 🟢) `import_runs` (`list_import_runs`, 🟢) `lead_signal_catalog` (`get_lead_signal_catalog`, 🟢) `legacy_loss_reasons` (`list_legacy_loss_reasons`, 🟢) `legal_entities` (`list_legal_entities`, 🟢) `legal_entity_members` (`list_legal_entity_members`, 🟢) `legal_entity` (`get_legal_entity`, 🟢) `legal_review_settings` (`get_legal_review_settings`, 🟢) `list_filters` (`describe_list_filters`, 🟢) `member_retirement` (`preview_member_retirement`, 🟢) `member` (`get_member`, 🟢) `my_ai_activity` (`get_my_ai_activity`, 🟢) `my_capture_activity` (`list_my_capture_activity`, 🟢) `my_email_signature` (`get_my_email_signature`, 🟢) `my_linkedin` (`get_my_linked_in_account`, 🟢) `notification_preferences` (`list_notification_preferences`, 🟢) `pipelines` (`list_pipelines`, 🟢) `provider_connections` (`list_provider_connections`, 🟢) `provider_locations` (`list_provider_locations`, 🟢) `query` (`describe_query_vocabulary`, 🟢) `record_fields` (`describe_record_fields`, 🟢) `report_blocks` (`describe_report_blocks`, 🟢) `report_vocabulary` (`describe_report_vocabulary`, 🟢) `retention_policies` (`list_retention_policies`, 🟢) `roles` (`list_roles`, 🟢) `saved_view` (`get_saved_view`, 🟢) `saved_views` (`list_saved_views`, 🟢) `user_handover` (`get_user_handover`, 🟢) `voice_build` (`get_voice_build`, 🟢) `voice_corpus_sources` (`list_voice_corpus_sources`, 🟢) `voice_learning_summary` (`get_voice_learning_summary`, 🟢) `voice_profile_deltas` (`list_voice_profile_deltas`, 🟢) `voice_profile_versions` (`list_voice_profile_versions`, 🟢) `voice_profile` (`get_voice_profile`, 🟢) `voice_profiles` (`list_voice_profiles`, 🟢) `web_form` (`get_web_form`, 🟢) `web_forms` (`list_web_forms`, 🟢) `worklist` (`get_worklist`, 🟢) `workspace_capture_activity` (`list_workspace_capture_activity`, 🟢) `workspace_email_domains` (`list_workspace_email_domains`, 🟢)</details> |
| `whoami` | 🟢 | `read` | no | Who this passport acts for |
| `draft_outbound` | 🟢 | `draft` | no | 🔀 Draft an email or follow-ups <details><summary>actions</summary>`one_message` (`draft_email`, 🟢) `slipping_deals` (`draft_follow_ups_for`, 🟢)</details> |
| `annotate_brief` | 🟢 | `write` | no | Write findings onto the morning brief |
| `archive_record` | 🟢 | `write` | no | Archive a record |
| `bulk_update_records` | 🟢 | `write` | no | Change many records at once |
| `change_access` | 🟢 | `write` | no | 🔀 Share records and set up members and teams <details><summary>actions</summary>`add_team_member` (`add_team_member`, 🟢) `assign_legal_entity` (`assign_legal_entity_member`, 🟢) `assign` (`create_record_assignment`, 🟢) `change_role` (`change_user_role`, 🟢) `claim_record` (`claim_record`, 🟢) `create_record_role` (`create_record_role`, 🟢) `create_team` (`create_team`, 🟢) `deactivate_member` (`deactivate_user`, 🟢) `end_assignment` (`archive_record_assignment`, 🟢) `hand_over_work` (`hand_over_user_work`, 🟢) `offer_ownership` (`offer_ownership_transfer`, 🟢) `reactivate_member` (`reactivate_user`, 🟢) `remove_member` (`remove_workspace_member`, 🟢) `remove_team_member` (`remove_team_member`, 🟢) `request_ownership` (`request_ownership_transfer`, 🟢) `share` (`share_record`, 🟢) `unshare` (`revoke_record_share`, 🟢) `update_assignment` (`update_record_assignment`, 🟢) `update_member` (`update_member`, 🟢) `update_record_role` (`update_record_role`, 🟢) `update_team` (`update_team`, 🟢) `withdraw_ownership_offer` (`withdraw_ownership_offer`, 🟢) `withdraw_ownership_request` (`withdraw_ownership_request`, 🟢)</details> |
| `change_activities` | dynamic | `write` | no | 🔀 Log, plan and file activities <details><summary>actions</summary>`accept_document_fields` (`accept_attachment_extraction`, 🟢) `archive_document` (`delete_attachment`, 🟢) `attach` (`attach_document`, 🟢) `bring_back` (`clear_activity_disposition`, 🟢) `cancel_send` (`cancel_scheduled_send`, 🟢) `check_new_send` (`preview_account_send_authorization`, 🟢) `check_send` (`preview_send_authorization`, 🟢) `discard_draft` (`discard_mail_draft`, 🟢) `hide_document` (`hide_deal_document`, 🟢) `log` (`log_activity`, 🟢) `preview_sign_off` (`preview_email_sign_off`, 🟢) `read_document_fields` (`read_attachment_for_fields`, 🟢) `read_transcript` (`read_transcript_for_next_steps`, 🟢) `ready` (`confirm_meeting_ready`, 🟢) `relink_many` (`relink_activities`, dynamic) `relink` (`relink_activity`, dynamic) `request_document_access` (`request_attachment_access`, 🟢) `reschedule_send` (`reschedule_scheduled_send`, 🟢) `save_draft` (`save_mail_draft`, 🟢) `set_aside` (`set_activity_disposition`, 🟢) `set_audience` (`set_activity_audience`, 🟢) `set_document_details` (`update_attachment_metadata`, 🟢) `set_thread_audience` (`set_thread_audience`, 🟢) `show_document` (`unhide_deal_document`, 🟢) `task` (`create_task`, 🟢) `unready` (`undo_meeting_ready`, 🟢)</details> |
| `change_attention` | 🟢 | `write` | no | 🔀 Act on proposals, notices and the weekly plan <details><summary>actions</summary>`accept_applied_change` (`accept_applied_deal_change`, 🟢) `accept_deal_suggestion` (`accept_deal_suggestion`, 🟢) `accept_tag_suggestion` (`accept_tag_suggestion`, 🟢) `acknowledge_failed_approval` (`acknowledge_approval_failure`, 🟢) `add_commitment` (`add_weekly_plan_commitment`, 🟢) `answer_teammate` (`answer_weekly_plan_commitment`, 🟢) `ask_for_help` (`ask_for_weekly_plan_help`, 🟢) `brief_item_done` (`mark_brief_item_acted`, 🟢) `dismiss_brief_item` (`mark_brief_item_dismissed`, 🟢) `dismiss_company_suggestion` (`dismiss_company_suggestion`, 🟢) `dismiss_contact_moment` (`dismiss_contact_moment`, 🟢) `dismiss_deal_move` (`dismiss_deal_move`, 🟢) `dismiss_deal_suggestion` (`dismiss_deal_suggestion`, 🟢) `dismiss_nudge` (`dismiss_relationship_nudge`, 🟢) `dismiss_nudges` (`dismiss_relationship_nudges`, 🟢) `dismiss_tag_suggestion` (`dismiss_tag_suggestion`, 🟢) `edit_commitment` (`edit_weekly_plan_commitment`, 🟢) `mark_all_notices_read` (`mark_all_notices_read`, 🟢) `mark_notice_read` (`mark_notice_read`, 🟢) `mark_rows_done` (`mark_worklist_rows_done`, 🟢) `pin_row` (`pin_worklist_row`, 🟢) `raise_notice` (`raise_notice`, 🟢) `reopen_rows` (`reopen_worklist_rows`, 🟢) `restore_company_suggestion` (`restore_company_suggestion`, 🟢) `restore_contact_moment` (`restore_contact_moment`, 🟢) `restore_deal_move` (`restore_deal_move`, 🟢) `restore_nudge` (`restore_relationship_nudge`, 🟢) `set_commitment_state` (`set_weekly_plan_commitment_state`, 🟢) `set_weekly_plan` (`set_weekly_plan_contract`, 🟢) `snooze_brief_item` (`snooze_brief_item`, 🟢) `start_weekly_plan` (`start_weekly_plan`, 🟢) `unpin_row` (`unpin_worklist_row`, 🟢) `unsnooze_brief_item` (`unsnooze_brief_item`, 🟢)</details> |
| `change_lists` | 🟢 | `write` | no | Make and change lists |
| `change_offers` | 🟢 | `write` | no | 🔀 Work offers, contracts and deal rooms <details><summary>actions</summary>`accept_offer` (`accept_offer`, 🟢) `add_room_document` (`add_deal_room_document`, 🟢) `archive_contract` (`archive_contract`, 🟢) `archive_room` (`archive_deal_room`, 🟢) `cancel_contract` (`cancel_contract`, 🟢) `close_room` (`close_deal_room`, 🟢) `pause_room` (`pause_deal_room`, 🟢) `record_contract` (`create_contract`, 🟢) `regenerate_offer` (`regenerate_offer`, 🟢) `reject_offer` (`reject_offer`, 🟢) `remove_room_document` (`remove_deal_room_document`, 🟢) `render_offer` (`render_offer`, 🟢) `renew_contract` (`renew_contract`, 🟢) `resolve_room_thread` (`resolve_deal_room_thread`, 🟢) `resume_room` (`resume_deal_room`, 🟢) `revoke_room_participant` (`revoke_deal_room_participant`, 🟢) `send_offer` (`send_offer`, 🟢) `set_contract_status` (`change_contract_status`, 🟢) `set_deal_products` (`replace_deal_products`, 🟢) `set_room_expiry` (`set_deal_room_expiry`, 🟢) `update_contract` (`update_contract`, 🟢) `update_room_document` (`update_deal_room_document`, 🟢) `update_room_participant` (`update_deal_room_participant`, 🟢) `update_room` (`update_deal_room`, 🟢)</details> |
| `change_record_parts` | 🟢 | `write` | no | Change the parts of a record |
| `change_relationships` | 🟢 | `write` | no | 🔀 Work a company's relationships and briefs <details><summary>actions</summary>`apply_employment_import` (`apply_contact_employment_import`, 🟢) `backfill_employment_import` (`backfill_employment_import`, 🟢) `cancel_intro_request` (`cancel_intro_request`, 🟢) `check_vat` (`request_company_vat_check`, 🟢) `complete_intro_request` (`complete_intro_request`, 🟢) `decide_intro_request` (`decide_intro_request`, 🟢) `draft_intro_note` (`draft_intro_note`, 🟢) `draft_intro_request` (`draft_intro_request`, 🟢) `link_contacts` (`link_company_contacts`, 🟢) `record_introduction` (`record_introduction`, 🟢) `record_promise` (`record_conversation_claim`, 🟢) `request_intro` (`create_intro_request`, 🟢) `restore_profile_field` (`restore_contact_profile_field`, 🟢) `save_research` (`save_contact_research`, 🟢) `settle_promise` (`settle_conversation_claim`, 🟢) `take_over_promise` (`take_over_conversation_claim`, 🟢)</details> |
| `change_reports` | 🟢 | `write` | no | 🔀 Write a draft of a saved report for a human to publish <details><summary>actions</summary>`draft` (`draft_report`, 🟢)</details> |
| `change_settings` | 🟢 | `write` | no | 🔀 Change how the workspace is set up <details><summary>actions</summary>`archive_legal_entity` (`archive_legal_entity`, 🟢) `ask_company_setup` (`message_onboarding_company`, 🟢) `ask_site_read` (`message_anchor_company_site_read`, 🟢) `confirm_site_read` (`confirm_anchor_company_site_read`, 🟢) `create_acquisition_source` (`create_acquisition_source`, 🟢) `create_disqualify_reason` (`create_lead_disqualify_reason`, 🟢) `create_lead_origin` (`create_lead_origin`, 🟢) `create_lead_source` (`create_lead_source`, 🟢) `create_legal_entity` (`create_legal_entity`, 🟢) `create_loss_reason` (`create_loss_reason`, 🟢) `decide_legacy_loss_reason` (`decide_legacy_loss_reason`, 🟢) `delete_disqualify_reason` (`delete_lead_disqualify_reason`, 🟢) `delete_lead_origin` (`delete_lead_origin`, 🟢) `delete_lead_source` (`delete_lead_source`, 🟢) `remove_legal_entity_logo` (`delete_legal_entity_logo`, 🟢) `remove_own_logo_icon` (`delete_anchor_company_logo_icon`, 🟢) `remove_own_logo` (`delete_anchor_company_logo`, 🟢) `save_booking_alias` (`put_scheduling_alias`, 🟢) `save_booking_page` (`put_scheduling_profile`, 🟢) `save_my_linkedin` (`save_my_linked_in_account`, 🟢) `save_own_company` (`put_anchor_company`, 🟢) `update_acquisition_source` (`update_acquisition_source`, 🟢) `update_disqualify_reason` (`update_lead_disqualify_reason`, 🟢) `update_duplicate_settings` (`update_contact_case_settings`, 🟢) `update_follow_up_settings` (`update_follow_up_settings`, 🟢) `update_lead_origin` (`update_lead_origin`, 🟢) `update_lead_settings` (`update_lead_settings`, 🟢) `update_lead_source` (`update_lead_source`, 🟢) `update_legal_entity` (`update_legal_entity`, 🟢) `update_legal_review_settings` (`update_legal_review_settings`, 🟢) `update_loss_reason` (`update_loss_reason`, 🟢) `update_review_template` (`update_activity_review_template`, 🟢)</details> |
| `change_state` | dynamic | `write` | no | 🔀 Move a lead, deal, project or partner <details><summary>actions</summary>`accept_partner_lead` (`accept_partner_lead`, 🟢) `clear_lead_signal` (`clear_lead_manual_signal`, 🟢) `correct_close_day` (`correct_deal_close_day`, 🟢) `decide_legal_review` (`decide_legal_review`, 🟢) `demote` (`demote_lead`, 🟢) `disqualify` (`disqualify_lead`, 🟢) `link_lead_company` (`link_lead_company`, 🟢) `merge_member_accounts` (`merge_members`, 🟡) `move_deal` (`progress_deal`, dynamic) `move_project` (`advance_project_phase`, 🟢) `promote` (`promote_lead`, 🟢) `propose_deal_roles` (`propose_deal_roles`, 🟢) `publish_contact` (`publish_captured_contact`, 🟢) `qualify` (`qualify_lead`, 🟢) `reopen_lead` (`reopen_lead`, 🟢) `request_legal_review` (`request_legal_review`, 🟢) `revert_stage` (`revert_stage_progression`, 🟢) `review_outcome` (`create_deal_outcome_review`, 🟢) `set_lead_signal` (`set_lead_manual_signal`, 🟢) `set_up_partner` (`set_up_partner`, 🟡) `withdraw_legal_review` (`withdraw_legal_review`, 🟢)</details> |
| `change_tags` | dynamic | `write` | no | 🔀 Change tags <details><summary>actions</summary>`apply` (`apply_tag`, 🟢) `archive` (`archive_tag`, 🟢) `create` (`create_tag`, 🟢) `merge` (`merge_tags`, 🟡) `remove` (`remove_tag`, 🟢) `restore` (`restore_tag`, 🟢) `update` (`update_tag`, 🟢)</details> |
| `commit_import` | 🟢 | `write` | no | Commit an import |
| `create_record` | 🟢 | `write` | no | Create a record |
| `decide_approvals` | 🟢 | `write` | no | 🔀 Approve or reject staged actions <details><summary>actions</summary>`bundle` (`decide_approval_bundle`, 🟢) `one` (`decide_approval`, 🟢)</details> |
| `preview_import` | 🟢 | `write` | no | Preview an import |
| `resolve_duplicates` | 🟢 | `write` | no | 🔀 Merge two records, or keep both <details><summary>actions</summary>`dismiss` (`decide_duplicate`, 🟢) `merge` (`merge_records`, 🟢)</details> |
| `update_record` | 🟢 | `write` | no | Update a record |
| `schedule_meeting` | dynamic | `send` | yes | 🔀 Book a meeting, or invite to it <details><summary>actions</summary>`invite` (`invite_meeting`, 🟡)</details> |
| `send_outbound` | 🟢 | `send` | yes | 🔀 Send a message <details><summary>actions</summary>`change_invitation` (`change_meeting_invitation`, 🟢) `chat_reply` (`send_message`, 🟢) `deal_room_invite` (`invite_deal_room_participant`, 🟢) `deal_room_resend` (`resend_deal_room_invitation`, 🟢) `email_new` (`send_company_email`, 🟢) `email_reply` (`send_email`, 🟢) `invite_member` (`invite_user`, 🟢) `meeting_proposal` (`create_meeting_proposal`, 🟢) `my_signature` (`save_my_email_signature`, 🟢) `signature_template` (`save_email_signature_template`, 🟢)</details> |
| `enrich` | 🟡 | `enrich` | yes | Look up a company on the web |
| `run_research` | 🟢 | `enrich` | no | 🔀 Run AI work for your user <details><summary>actions</summary>`ask_about_company` (`ask_about_company`, 🟢) `generate_brief` (`generate_morning_brief`, 🟢) `refresh_company_brief` (`regenerate_company_brief`, 🟢) `refresh_contact_brief` (`regenerate_contact_brief`, 🟢) `refresh_dossier` (`refresh_company_dossier`, 🟢) `refresh_growth_fit` (`refresh_company_growth_fit`, 🟢) `refresh_scan` (`ensure_company_scan`, 🟢) `research_contact` (`run_contact_research`, 🟢)</details> |

## Rules for single tools

- **`update_record` is 🟢.** The whole patch applies in the one call, whoever last wrote the fields. The audit row and
  Undo put a wrong edit back. A call the contract marks confirm-first stages the whole call instead.
- **A write says what it changed.** `create_record` and `update_record` list each field they set in `changes`. Each
  entry has the field's label and its value before and after, with a name in place of an ID. The approval inbox shows a change in the same words. `read_approval` carries the
  same `changes` for a change that waits in the inbox.
- **A deal move follows its stage.** It waits or runs by what the stage means. A column renamed `"Won"` still waits.
  `advanceDealTier` trusts the meaning set up for the stage. A stage it cannot prove is `open` waits too.
- **Margince works out the list filters at boot.** `find_records` with `engine=list` takes them. They are the overlap
  of the `crm.yaml` query fields of each list operation and what the record store can bind.
### Why a tool waits

- A deal move to an `open` stage runs; a move to a `won` or `lost` stage waits.
- The relink verbs (`relink_activity`, `relink_thread`, `relink_activities`) read the record type the activity moves
  to. Filing under a project is 🟡, because it marks every named activity as sales mail. A thread or a named set
  cannot pin a version, so a named set always waits.
- `relink_thread` refuses every call; `relink_activities`, whose approval binds the named IDs, replaces it.
- A credential may give the approval for its own staged relink with `decide_approvals`, on a call with a human there.
  The same checks the decision applies must pass: the decision grants, and the target is visible. The change must
  also be one a user can undo. Margince records the approval as the decision of the member, made through the agent.
- A member takes a filing under a project back with *Undo filing*, a `human-only` decision whose written reason is
  never the reason of the credential. So a credential may give the approval for a project relink only while that
  undo could still apply.
- It may not when the activity is limited, archived, under a legal hold or covered by an open request to erase. Nor
  when Margince keeps it for another reason: a `won` deal, a sent offer, a pin by the data controller. A member gives
  the approval for such a relink in the CRM.
- **Why the rest run at once.** A tool is 🟢 when the human who gave the passport could do the same act alone. The tier
  with the last word is `agentPolicies` in `compose/agentpolicy_gen.go`, generated from `crm.yaml`.

## Reads filtered to what you may see

`resolve_entities` and `search_context` answer only records the caller may see, from engines that see more.
- The dedupe ladder behind `resolve_entities` looks across the whole workspace. A copy is a copy, no matter who
  looks. A match set cut down per caller would let one payload create a second record for one rep and not for
  another.
- Every ID it names is read back through the datasource seam before Margince serves it. That read applies object
  RBAC and row scope, marks the trust tier, and counts the record as served on the answer.
- A match the caller may not read answers `unresolved`, the same word a real miss gets. So a caller cannot test one
  address at a time for records they may not know exist.
- An `ambiguous` answer stays `ambiguous` when only one other match is still there. So what the caller cannot see
  never decides a question.
- Margince reports the filtering once per call, without a count.

## What each scope buys

The passport words are a closed set: `read`, `draft`, `write`, `send`, `enrich` (`principal.Scope`). What an agent may
really do is always the overlap of two things. One is the passport scopes. The other is the live RBAC and seat of the
human who gave the passport. It is never both added up, and never the passport alone.

| Scope | What it means |
|---|---|
| `read` | Reads only. It is also the only scope that makes a tool `readOnlyHint: true`, and the only scope a **read seat** may spend at all. |
| `draft` | Writes a draft. Not read-only: `draft_email` leaves a first message in the drafts of the human it acts for (never on a timeline; a reply it writes in no place). `draft_follow_ups_for` stores a draft activity on the deal timeline. |
| `write` | Creates, patches, archives, moves on, merges, moves leads up or out, links again: every change that stays inside the workspace. |
| `send` | Egress verbs: they reach outside the workspace. `invite_meeting` is 🟡. The others run at once, on the rights of the human who gave the passport. |
| `enrich` | `enrich`, which gets data from a third party, 🟡 and `Egress: true`: the cap buys the right to ask. `run_research`, AI work that stays inside Margince and runs at once with no approval; only its `research_contact` action asks an outside provider. |

The live count per scope is in [mcp-info.md](mcp-info.md).

The `enrich` cap holds the company routes on REST, `scrapeCompany` (`POST /v1/companies/{id}/enrich`), `deepReadCompany`
(`POST /v1/companies/{id}/deep-read`) and `technicalEnrichCompany` (`POST /v1/companies/{id}/technical-enrich`), and the
`enrich` tool built on them on `/mcp`. A passport is
a Bearer credential for `/v1`, under the same rules as `/mcp`. The cold-start routes are `human-only`: they create the
company and do not enrich one. Grant the cap when the agent's job is research outside the CRM on a record that exists.

## Operations an agent may not reach at all

`api/crm.yaml` marks operations that change data with `x-mcp-tool`. `tools/gen-agentpolicy` compiles those marks into
`internal/compose/agentpolicy_gen.go`, the table the REST admission gate reads. An operation that no tool can back
carries `x-agent-access: human-only` instead, and the gate refuses an agent principal, no matter its scope or seat.
Many operations carry it. These are the ones an agent may expect to reach:

| Operation | Why no agent may call it |
|---|---|
| `coldStartReadback`, `coldStartPreview` | They *create* the company, so there is no record for a verb that works on a record to point at. The `enrich` tool keeps the company routes. |
| `listRecordGrants` | Who was given a record by hand is an access read, which stays human-only. An agent shares a record and takes a share back through `change_access`. |
| `createCustomField`, `renameCustomField`, `updateCustomFieldOptions`, `retireCustomField` | A custom field is a definition the whole workspace shares, down to its label. An agent reads the live fields through `describe_vocabulary` (action `describe_query_vocabulary`, the verb `listCustomFields` names). |
| `createWebhookSubscription`, `updateWebhookSubscription` | A subscription decides where events leave the installation. Every other webhook operation is `human-only` too. |
| `createSavedView`, `updateSavedView`, `archiveSavedView` | A saved view is one member's own screen. An agent reads the saved views through `read_workspace` (topics `saved_views` and `saved_view`). |

An intent tool built over many operations, such as `catch_me_up_on` or `progress_deal`, has a route of its own. That
route runs the tool through the registry (`served_by: registry`), so the verb is declared like any other. Its
`OpenAPIOp` names that route.

## One passport, the same functions on REST and MCP

Every operation a passport may call on `/v1` names, in `x-mcp-tool`, the MCP verb that does the same thing.
That verb takes the route's record type and every one of its query and path parameters.

Two gates hold this. `TestEveryPolicyRecordTypeIsOneItsToolCanSend` fails a route whose verb refuses its record type.
`TestEveryAgentRouteParameterIsAnArgumentOfItsVerb` fails a route parameter the verb has no argument for,
apart from the page size, which MCP keeps smaller. A nested route, such as `/deals/{id}/offers`, names its
parent in the path. The record tools take the same parent as a filter or a field (`deal_id`).

## What a live server can show differently

The catalog comes from the `mcp.ToolSpec` each tool returns from `Spec()`. **The running server has the last word**, and what a live server shows can differ from the tables:

- **`tools/list` is filtered by scope for each caller.** The dispatcher drops each tool whose `RequiredScope` the
  passport does not hold (`invocableByCaller` in `backend/internal/modules/agents/dispatch.go`). So an agent made with
  `["read"]` sees the read tools and nothing else. The filter copies the scope check of the admission gate, so the
  list never shows what the gate will refuse.

  It answers **only for scope**. Each call checks the seat limit and object RBAC of the human who gave the
  passport. They can still refuse a tool the list showed.
- **Extensions register after core.** `registerComposedTools` runs last in `internal/compose/registry.go`, so an
  extension unit can add verbs, and a name that is the same as a core verb fails boot. Each extension operation
  declares `x-mcp-tool` or `x-agent-access: human-only`, the same closed set of options core operations have. A
  `human-only` operation stays open to REST and the UI, refuses an Agent (or Buyer) principal, and never shows in
  `tools/list`.

  A tool that must be confirmed first must declare `x-mcp-tool.subject`. At boot, Margince refuses `send` and `enrich`
  from an extension; the rules are in [how-to/add-an-extension.md](../how-to/add-an-extension.md). The composed set is
  a trust line, and that limits what the handler of a unit can do.
- **The plain tree adds no tools.** `extensions/de` and `extensions/vn` register none, and every operation of
  `extensions/openchannel` is `human-only`. On a plain install, the catalog below is all an agent can reach.

`TestTheToolCatalogsTiersAreTheContractsTiers` holds the tiers and rows of the tables against the contract, both ways,
and `TestEveryDeclaredToolVerbIsRegistered` and `TestEveryRegisteredToolIsDeclared` hold the verbs, both ways. The gates
for scopes and hints are in [mcp-protocol.md](mcp-protocol.md#how-the-tool-hints-are-set).

What an agent reads when a tool refuses is in [mcp-protocol.md](mcp-protocol.md#when-a-tool-refuses).

## Where to go next

- What a human may do, which limits every agent: [rbac-matrix.md](rbac-matrix.md).
- What the `agents` module owns and where it sits: [modules.md](modules.md).
- The transport, limits and client sign-in: [mcp-protocol.md](mcp-protocol.md).
