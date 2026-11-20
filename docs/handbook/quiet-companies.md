<!-- prose:plain -->
<!-- describes: backend/internal/shared/kernel/dormancy backend/internal/compose/company360/dormancy.go backend/internal/modules/contacts/dormancysettings.go frontend/src/features/companies/companyactivity.ts -->
# Companies that have gone quiet

Margince shows how long it has been since anything happened with a company, and suggests getting
back in touch with one that has gone quiet. Who here knows a contact, and how warm that is, is in
[Relationships, introductions and research](relationships-and-research.md).

### How cold has a company gone?

The **How it stands** card on a company says how long it has been since anything happened with
the company, until someone rates it. It counts calendar days since the company's last activity:

- **Active**: under 30 days.
- **Cooling**: from day 30.
- **Dormant**: from day 60.
- **Stale**: from day 90, and also a company with no activity recorded at all.

The line under the word gives the number of days, the date of the last activity and the three days
of the rule.

Mail that a colleague holds as personal does not count. Neither does a message on a deal that was
marked as not about that deal. It still counts when it is also filed on the company or on one of
its people.

On **Companies**, the **Activity** filter lists the companies on one of these steps.

An administrator can change the three days in the configuration, as
`contacts.dormancy_days`. They must go up, such as 30, 60 and 90.

Also called: cold accounts, gone quiet, account health, last contact.

### When does a company page suggest getting back in touch?

When a company is on **Dormant** or **Stale**, its page suggests **Get back in touch with** the company. **Add
next step** adds that task for the company's owner.

- A company with no owner gets no suggestion. Margince does not hand the task to anyone else.
- A company with no activity at all gets no suggestion, because it never went quiet.
- **Not now** hides the suggestion until there is new activity and the company goes quiet again.
- An administrator can switch the suggestion off with `contacts.dormancy_follow_up_enabled`. The
  steps on the card stay.

Also called: win back a customer, follow up a quiet account.
