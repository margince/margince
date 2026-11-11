<!-- prose:plain -->
<!-- describes: frontend/src/features/analytics/progress.tsx frontend/src/features/analytics/goals.tsx frontend/src/features/analytics/goals.editor.tsx backend/internal/compose/salesprogress backend/internal/modules/reporting/goalplans.go -->
# Progress and sales goals

## Where do I see my results against my goal?

Open **Insights → My progress**. A manager sees **Team progress** there instead. A rep sees their own progress. A manager can pick a seller or a team under **Seller or team**. Pick **Monthly** or **Yearly**, then the month or the year.

The top panel shows closed revenue for the period up to the cutoff named above it. Closed revenue counts for the deal owner of today, so a won deal that gets a new owner moves to that owner's progress. Below it is the goal for the period. When both are complete you also see:

- how much of the goal is reached, in %. The % number keeps going past 100%.
- the difference to the goal: above it, below it in red, or **Goal met** when the two are the same.

A goal of zero shows only the difference, without a %.

The bar beside the figure shows closed revenue in its parts, with a line where the goal is. After the parts, a light box with a broken line shows planned revenue that may still close.

Click **View closed deals** to open the won deals behind the figure.

Under the result, **Product sales** and **Services and projects** split closed revenue by what each deal is for. The split comes from the products on the deal and the kind of each product. **Planned revenue** is the value of open deals expected to close in the period. It is not weighted by probability and never counts for the goal. **Unclassified** appears when some revenue has no place. For example, a deal has no lines, or its lines add up to more than the deal value.

A team view lists each seller in name order. Click a name to open the progress of that seller for the same period. Each row has a bar that shows how much of their goal that seller has reached.

**Work in progress now** lists what is open today, whatever the period. **Leads** has a tab for each open status with its count.

**Deals** shows each open deal with its company, stage and planned amount. A label says what it sells: **Product**, **Service** or both. It says **Partly classified** or **Unclassified** when the lines do not place it all. A close date that has passed says **Overdue**. A date in another period says **Outside the period**, and a deal with no date says **No date**.

The next task shows its due date, and the name of the colleague it is for when that is not the deal owner. A deal or lead with no open task says **No next task**.

The months show as a chart of each goal with its closed revenue inside it, and as a table under it. A month that has not started says **Not started** and has no difference.

## Why is a goal not complete?

A yearly goal needs all 12 months. If some months have no goal, Progress shows the months that are set and their sum so far, without a % or a difference. In a team, a month counts as set only when every seller has a goal for it.

Other states you may see:

- **No goal set**: no month in the period has a goal. A missing goal is not a goal of zero.
- **In another currency**: the goal was set in a currency other than the reporting currency. It is never converted.
- **Changed outside the plan**: someone changed a published goal from another place. Months still compare; the year waits until a manager reviews the plan.

## How does a manager set sales goals?

Open **Insights → Sales goals** and pick the year. A manager picks a seller from the list beside the plan; each one shows the published yearly goal and the state of the plan. The 12 months show as bars above the amounts. Reps can read their own goals here but cannot change them. A rep can also read a draft and the reason for each change.

1. Type the yearly goal and click **Spread equally**, or type each month. **Spread equally** gives each month the same part of the year.
   If the year does not split evenly, the first months get one cent more each. In other currencies it is their smallest unit.
2. Write a reason and click **Save draft**. A draft changes nothing in Progress until it is published.
3. Click **Publish**, confirm that the goals were agreed together, and publish. The months now count in Progress.

You can only publish a saved draft whose 12 months are all set and add up to more than zero. If you typed a yearly goal, the months must also add up to it. **Discard draft** removes the draft and keeps what is published; it asks for no reason. **Withdraw** removes published goals; it asks for a reason. Every step is kept in the plan history with who did it and when.

Once a plan publishes the months of a seller, those months can only be changed through **Sales goals**.
