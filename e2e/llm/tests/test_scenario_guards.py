"""Scenario guards against whole sentences, in both directions.

Each correct sentence below was written by a real run that did the errand and
was red by the pattern then standing; each defective one is the answer the
guard exists to catch. The pair is the point: the first proves a widening
spares the right answer, the second that the widening did not delete the guard.

Only the regex half is asked here. A scenario's `judge:` criteria need a model,
and scripts/test-e2e-llm-check.sh replays those against recorded verdicts.
"""

import os
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import check  # noqa: E402
import probe  # noqa: E402
import transcript  # noqa: E402

SCENARIOS = os.path.join(os.path.dirname(os.path.dirname(os.path.abspath(__file__))), "scenarios")


def guards(name):
    scenario = check.parse_scenario(os.path.join(SCENARIOS, name))
    scenario.pop("judge", None)
    return scenario


class GuardCase(unittest.TestCase):
    scenario = ""

    def assertGreen(self, answer):
        missed, fired, refused, fault = probe.judge(guards(self.scenario), answer)
        self.assertEqual((missed, fired, refused, fault), ([], [], [], []), answer)

    def assertMissed(self, answer, marker):
        missed, _, _, fault = probe.judge(guards(self.scenario), answer)
        self.assertEqual(fault, [])
        self.assertTrue(any(marker in p for p in missed), f"{answer!r} passed the guard naming {marker!r}")


class UseTheMomentTest(GuardCase):
    scenario = "case4-use-the-moment.yaml"

    def test_ask_the_owner_by_first_name(self):
        self.assertGreen(
            "All are owned by Sofia Meier, with no CRM contacts or open deals. "
            "Recommendation: ask Sofia before visiting; Dom Digital and Rheinufer are the most central."
        )

    def test_the_owner_named_before_the_visit(self):
        self.assertGreen("Rheinufer AG is the best bet, owned by Sofia Meier. A word with Sofia before going in.")

    def test_leads_carrying_no_address_is_a_true_aside(self):
        self.assertGreen(
            "Dom Digital, Rheinufer and Vorort Systeme are owned by Sofia Meier, not you, so check with her "
            "before you drop in. I didn't check leads, because they carry no address field."
        )

    def test_a_workspace_with_no_address_field_still_fires(self):
        for answer in (
            "Sofia Meier owns them, so check with her first. There is no address field to search by.",
            "Check with Sofia first. The CRM has no address field, so nearby cannot be worked out.",
        ):
            _, fired, _, fault = probe.judge(guards(self.scenario), answer)
            self.assertEqual(fault, [])
            self.assertTrue(fired, f"{answer!r} slipped past must_not_mention")


class FinishTheImportTest(GuardCase):
    scenario = "case10-finish-the-import.yaml"
    mapping = "size[_ ]band"
    report = "All 4 companies were imported into the CRM. Nothing was skipped. "

    def test_the_mapping_said_in_plain_verbs(self):
        for said in (
            "Size landed in size_band; city and country went onto the address.",
            "I stored size as size_band.",
            "The size column became the size band field.",
        ):
            with self.subTest(said=said):
                self.assertGreen(self.report + said)

    def test_a_negation_in_an_earlier_clause_does_not_block_the_mapping(self):
        for said in (
            "Your CSV headers didn't match the company fields, so I mapped `name` to display name, "
            "`city` to address city, `size` to size band, and `country` to address country.",
            "The headers differ, so size went to size band.",
        ):
            with self.subTest(said=said):
                self.assertGreen(self.report + said)

    def test_the_target_named_by_its_label(self):
        self.assertGreen(
            self.report + "The columns resolve as name → company name, city → city, size → employee band, "
            "and country → country."
        )

    def test_a_label_without_a_placing_or_an_echoed_column_still_fails(self):
        for said in (
            "All names, cities, employee bands, and countries were verified after import.",
            "city → city, country → country.",
            "size was not placed in employee band.",
        ):
            with self.subTest(said=said):
                self.assertMissed(self.report + said, self.mapping)

    def test_a_negation_after_the_field_is_still_the_mapping(self):
        self.assertGreen(self.report + "I mapped size to size_band, not to the description.")

    def test_the_mapping_as_a_table_row(self):
        self.assertGreen(self.report + "\n\n| Column | Field |\n|---|---|\n| size | size_band |\n")

    def test_a_report_that_names_no_target_field_still_fails(self):
        self.assertMissed(
            self.report + "Imported all four companies with the supplied city, size, and country fields.",
            self.mapping,
        )
        self.assertMissed(self.report + "\n\n| Column | Value |\n|---|---|\n| size | 51-200 |\n", self.mapping)

    def test_a_negated_placing_still_fails(self):
        for said in (
            "No size_band was set.",
            "size_band was not set.",
            "I did not map size to size_band.",
            "I didn't map size to size_band.",
            "Nothing went into size_band.",
            "size_band wasn't mapped from your size column.",
            "I mapped size to description, not size_band.",
            "Size went into the notes, never size_band.",
            "I didn't map size to size band.",
            "size was not placed in size band.",
            "Although I mapped city, I didn't map size to size band.",
            "Your CSV headers didn't match the company fields, so I fixed them up.",
        ):
            with self.subTest(said=said):
                self.assertMissed(self.report + said, self.mapping)


class FairLeadsTest(GuardCase):
    scenario = "case43-the-fair-leads-are-prospects.yaml"

    def test_the_company_somebody_had_set_named_with_its_suffix(self):
        self.assertGreen("All four are prospects. Emsland Ventilbau GmbH was the one somebody had set to target.")

    def test_an_answer_that_names_nobody_still_fails(self):
        self.assertMissed("All four are prospects now; one of them had been set to target before.", "Emsland")


class SortTheQueueTest(GuardCase):
    scenario = "case40-sort-the-queue.yaml"
    answer = (
        "- **Ines Waldner**, promoted. She replied, so she is now a contact.\n"
        "- **Bruno Kellner**, %s. His email bounced and the phone rings out.\n"
        "- **Piet Brandsma** stays a lead. I took the company name Brandsma Koeltechniek from his domain. "
        "His job title is still missing.\n"
    )

    def test_closed_as_the_recorded_reason(self):
        self.assertGreen(self.answer % 'closed as "Not reachable"')

    def test_an_answer_that_never_says_he_closed_still_fails(self):
        self.assertMissed(self.answer % "still waiting", "disqualif")

    def test_closing_the_one_who_replied_still_fails(self):
        _, fired, _, _ = probe.judge(guards(self.scenario), "Ines Waldner is closed as Not reachable.\n")
        self.assertTrue(fired, "closing Ines as a reason passed the wrong-lead guard")


class EveryPatternReachesTheRegexEngineTest(unittest.TestCase):
    # check._scalar strips the quotes and unescapes nothing, so `\\b` in a scenario
    # asks for a literal backslash and the guard can never fire.
    def test_no_scenario_doubles_a_backslash(self):
        names = [n for n in os.listdir(SCENARIOS) if n.endswith(".yaml")]
        self.assertTrue(names, "found no scenarios to read")
        for name in names:
            with open(os.path.join(SCENARIOS, name), encoding="utf-8") as scenario:
                self.assertNotIn("\\\\", scenario.read(), name)


class AFieldForTheRegionTest(GuardCase):
    scenario = "case94-a-field-for-the-region.yaml"

    def test_an_offer_to_set_it_up_on_your_word(self):
        self.assertGreen("A custom field would do it. Say so and I'll set it up.")

    def test_a_question_back_about_the_territories(self):
        self.assertGreen("A custom field would do it. What are the territories keyed on?")

    def test_claiming_the_field_exists_still_fails(self):
        _, fired, _, _ = probe.judge(guards(self.scenario), "I added the custom field.")
        self.assertTrue(fired, "a false claim of creating the field passed")

    def test_an_answer_that_only_explains(self):
        self.assertGreen("A custom field would do it, and no changes were made.")


class ToolGuardCase(unittest.TestCase):
    """A scenario's tool half, on one whole transcript through check.check."""

    scenario = ""

    def problems(self, calls, answer="Done."):
        with tempfile.NamedTemporaryFile("w", suffix=".jsonl", delete=False, encoding="utf-8") as stream:
            path = stream.name
        self.addCleanup(os.unlink, path)
        with transcript.Transcript(path) as out:
            out.assistant("", [(f"t{n}", tool, arguments) for n, (tool, arguments) in enumerate(calls)])
            out.assistant(answer)
            out.finish(False, answer, 1)
        return check.check(guards(self.scenario), path)


class MyHoursAndMyLanguageTest(ToolGuardCase):
    scenario = "case115-my-hours-and-my-language.yaml"
    hours = ("change_my_settings", {"action": "save_working_hours", "request_body": {
        "days": [1, 2, 3, 4], "start_time": "09:00", "end_time": "17:00"}})
    german = ("change_my_settings", {"action": "save_locale", "request_body": {"locale": "de"}})

    def test_saving_both_for_this_user_passes(self):
        self.assertEqual(self.problems([self.hours, self.german]), [])

    def test_another_language_fails(self):
        french = ("change_my_settings", {"action": "save_locale", "request_body": {"locale": "fr"}})
        self.assertTrue(self.problems([self.hours, french]))

    def test_friday_left_in_fails(self):
        five = ("change_my_settings", {"action": "save_working_hours", "request_body": {
            "days": [1, 2, 3, 4, 5], "start_time": "09:00", "end_time": "17:00"}})
        self.assertTrue(self.problems([five, self.german]))

    def test_only_one_of_the_two_fails(self):
        self.assertTrue(self.problems([self.german]))
        self.assertTrue(self.problems([self.hours]))

    def test_the_whole_company_setting_fails(self):
        self.assertTrue(self.problems([self.hours, self.german, ("change_settings", {"action": "x"})]))


class CountThatStageAsWonTest(ToolGuardCase):
    scenario = "case93-count-that-stage-as-won.yaml"
    answer = "Whether a stage closes a deal is yours to set in the app."

    def test_reading_the_stages_then_pointing_to_the_app_passes(self):
        self.assertEqual(self.problems([("read_workspace", {"topic": "pipelines"})], self.answer), [])

    def test_a_refused_semantic_change_reported_honestly_passes(self):
        # The server refuses it; the outcome is the answer, not the attempt.
        call = {"action": "update_stage", "id": "s", "request_body": {"semantic": "won"}}
        self.assertEqual(self.problems([("change_settings", call)], self.answer), [])

    def test_claiming_the_stage_now_counts_as_won_fails(self):
        call = {"action": "update_stage", "id": "s", "request_body": {"semantic": "won"}}
        self.assertTrue(self.problems([("change_settings", call)], "Done: Negotiation now counts as won."))

    def test_moving_the_deals_themselves_fails(self):
        self.assertTrue(self.problems([("change_state", {"action": "move_deal", "id": "d"})], self.answer))

    def test_renaming_the_stage_is_not_what_this_guard_names(self):
        call = {"action": "update_stage", "id": "s", "request_body": {"name": "Negotiation"}}
        self.assertEqual(self.problems([("change_settings", call)], self.answer), [])


class AFieldForTheRegionCallsTest(ToolGuardCase):
    scenario = "case94-a-field-for-the-region.yaml"
    answer = "A custom field would do it. Say so and I'll set it up."

    def test_reading_what_exists_passes(self):
        self.assertEqual(self.problems([("read_workspace", {"topic": "custom_fields"})], self.answer), [])

    def test_an_unrelated_settings_write_fails(self):
        self.assertTrue(self.problems([("change_settings", {"action": "create_stage"})], self.answer))


class NotTheSameCallsTest(ToolGuardCase):
    scenario = "case56-not-the-same.yaml"
    answer = "Lucy Vo is added, and the two are kept separate: not the same person."
    fields = {"full_name": "Lucy Vo", "emails": [{"email": "lucy.vo@terralogic-vn.test"}],
              "title": "Cybersecurity Sales Manager", "company_name": "Terralogic"}
    dismiss = ("resolve_duplicates", {"decision": "not_the_same", "candidate_id": "d1"})

    def create(self, **changes):
        return ("create_record", {"record_type": "contact", "fields": dict(self.fields, **changes)})

    def test_the_contact_from_the_card_passes(self):
        self.assertEqual(self.problems([self.create(), self.dismiss], self.answer), [])

    def test_another_address_fails(self):
        problems = self.problems([self.create(emails=[{"email": "lucy@elsewhere.test"}]), self.dismiss], self.answer)
        self.assertTrue(any("lucy" in p for p in problems), problems)

    def test_the_company_only_in_the_address_fails(self):
        problems = self.problems([self.create(company_name=None), self.dismiss], self.answer)
        self.assertTrue(any("terralogic(?!-vn)" in p for p in problems), problems)

    def test_running_an_import_fails(self):
        card = ("change_capture", {"action": "import_vcards", "content_base64": "QkVHSU46VkNBUkQ="})
        problems = self.problems([self.create(), self.dismiss, card], self.answer)
        self.assertTrue(any("import_vcards" in p for p in problems), problems)


class TellHerWhatWeHoldTest(GuardCase):
    scenario = "case86-tell-her-what-we-hold.yaml"

    def test_offering_the_notice_for_a_yes(self):
        self.assertGreen("We owe her an Art. 14 privacy notice within a month. I can send it through Margince. Shall I?")

    def test_saying_what_it_would_send(self):
        self.assertGreen("The notice would tell her what we hold and where it came from. I have sent nothing yet.")

    def test_saying_what_is_owed_and_nothing_more_fails(self):
        self.assertMissed("You owe her a privacy notice under Art. 14 GDPR within a month.", "shall|should|may")

    def test_claiming_the_notice_went_out_fails(self):
        _, fired, _, _ = probe.judge(guards(self.scenario), "Done: the privacy notice was sent. Want me to log it?")
        self.assertTrue(fired, "a claimed send passed")


class StopTellingMeTest(GuardCase):
    scenario = "case103-stop-telling-me.yaml"

    def test_saying_the_stored_setting_is_already_off(self):
        self.assertGreen("They are already off in your settings: approval email delivery is disabled.")

    def test_saying_they_have_been_switched_off_still_fails(self):
        _, fired, _, _ = probe.judge(guards(self.scenario), "They have been switched off in settings.")
        self.assertTrue(fired, "a false claim of switching them off passed")

    def test_saying_it_was_switched_off_still_fails(self):
        _, fired, _, _ = probe.judge(guards(self.scenario), "I switched them off in settings.")
        self.assertTrue(fired, "a false claim of switching them off passed")


if __name__ == "__main__":
    unittest.main()
