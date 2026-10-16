<!-- prose:plain -->
# Configuration

The **Configuration** page lists every setting the installation reads, what its value is and where that value comes from. It is read-only. It sits in the **Governance** group of Settings, and only users who may change installation settings see it, which Admin and Ops do.

### What does the Configuration page show?
Each row shows the setting's **Key**, its **Value**, its **Source** and the **Environment variable** that sets it. Open a row to read what the setting does, its default, its type and the command-line flag that sets it.

Type in the filter box to narrow the list by key or by environment variable. If nothing matches, the page says so.

### Where does a value come from?
The **Source** column says which of these set the value:
- **Default value**: nothing set it, so the built-in default applies.
- **Saved in the app**: an administrator saved it in Settings.
- **Set by a flag, an environment variable, a `.env` file or a configuration file**: whoever runs the installation set it outside the app. The column names the flag or variable, and the file when there is one.

### How do I change a setting set outside the app?
Change it where its source says. A setting set by a flag, an environment variable or a file is locked in the app, and this page does not edit it. The same name and place show beside the locked control on its own Settings page.

### Why does a row show "Secret" or "Not set"?
A secret shows the word "Secret" and never its value or its default. A setting that only the background worker or the migration program reads shows "Not set", because the app cannot see the sources of those programs. It does not mean the setting is empty.

For the other settings pages, see [Settings](settings.md).
