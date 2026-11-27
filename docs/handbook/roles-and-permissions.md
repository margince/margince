<!-- prose:plain -->
<!-- describes: frontend/src/features/settings/roles-settings.tsx frontend/src/features/settings/roles-detail.tsx frontend/src/features/settings/roles-drawer.tsx frontend/src/features/settings/grantmatrix.tsx frontend/src/lib/roles.queries.ts backend/internal/modules/identity/rolelifecycle.go backend/internal/modules/identity/roles.go frontend/src/features/sign-in/group-role-grants.tsx frontend/src/features/sign-in/google-directory.tsx backend/internal/modules/identity/googledirectory.go -->
# Roles and permissions

The **Roles and permissions** page in Settings, under **People**, lists every role your company has. Open a role to see what its members may do, and whose records they may change.
This page is for the screen. For what each built-in role allows, and how row scope works, see [Seats, roles and who can see what](seats-roles-and-access.md).

### Who can open this page, and who can change a role?
<!-- opens: settings/roles -->
You need the right to read roles to see the page. If your role can read roles but not change them, the page says "Your role can read roles but not change them." The page is then read only.

Changing is split by what it does:
- Rename a role, turn a right off, or narrow whose records it reaches: any member whose role may change roles.
- Archive a custom role: any member whose role may delete roles.
- Create or restore a role, turn a right on, or widen whose records a role reaches: an Admin only. Without the Admin role, the controls stay off, and the server refuses the change too.

### What does the list show?
Each row is one role. It shows the role's name, a line on whose records its members change, and how many members hold it. The member count shows only if you may read members.
Badges mark each role as **Built-in**, **Custom** or **Archived**.
Custom roles come first, then the built-in ones.

By default the list hides archived roles. Turn on **Show archived roles** to see them. An archived role grants nothing and cannot be given to anyone.

### How do I make a new role?
An Admin chooses **New role**. A role always starts as a copy, so there is no blank role.
1. Under **Copy rights from**, pick the role to copy. The new role starts with all of its rights.
2. Type a **Name**, up to 120 characters.
3. Choose **Create role**.

The new role opens at once. If another role has the name, Margince refuses it and the name stays as you typed it.

### What can I change on a role?
Press a role's row to open it. A change applies to every member holding the role from their next request.
- **Rename**: change the name. Choose **Save name**.
- **Whose records members change**: **Their own**, **Their teams’** or **Everyone’s**. Only an Admin can pick a wider choice than the role has now. Anyone who may change roles can pick a narrower one. Every role that may read contacts, companies, leads, deals and projects reads all of them. This choice decides which ones members may change.
- **Permissions**: one row for each kind of record or setting. Each has up to four switches: **Read**, **Create**, **Update** and **Delete**. Each switch saves at once. **Create**, **Update** and **Delete** need **Read** on the same row, so turn **Read** on first.
- **Show API names**: shows the technical name beside each row.
- **Preview what this role sees**: opens a preview of what a member of this role reaches. Margince asks the server for it when you open it.

Extension rows appear in their own group, named for the extension. Rights that fit no group sit under **Other**.

A role that cannot connect a mailbox shows a note saying so. Mailboxes its members already connected stop capturing.

If someone else changed the role while you had it open, Margince shows their change and asks you to make yours again.

### How do I archive or restore a role?
Open the role, then choose **Archive role** or **Restore role**.
- A built-in role cannot be archived.
- A role cannot be archived while members who can sign in still hold it. Give them another role first.
- Only an Admin restores a role.
- An archived role is read only. Restore it to change it.

The Admin role keeps its rights to manage users, roles and teams. Margince refuses a change that would remove them, so an Admin can always run your company.

### How do I put a built-in role back as it shipped?
Open a built-in role and choose **Restore shipped grants**, then confirm. Every switch goes back to what Margince ships, and your changes to that role's rights are lost. A custom role has no shipped rights, so it has no such button.

Also called: permissions, access rights, create a role, custom role, who can edit what.

### How do I give a role to everyone in a group?
To give a role to everyone in a group, open Settings → **Sign-in and security** → **Group role grants**. Choose **Add group**, type the group and pick its role. Then choose **Save group roles**.
A member gets the role at their next sign-in. The map only adds roles, so leaving a group takes none away. Nobody gets in without an invitation.

Microsoft sends a member's groups at sign-in, so type a group as your company names it. Google does not. A member who signs in with Google gets roles only after you connect the **Google group lookup** on the same tab.
Google names a group by its email address, such as `sales@example.com`. Only groups the member belongs to count, not groups inside other groups.

If the lookup fails, the member still signs in with no roles from the map, and the tab says what to fix.

Also called: group roles, company groups.
