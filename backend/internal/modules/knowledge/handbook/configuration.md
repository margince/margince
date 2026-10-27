<!-- prose:plain -->
<!-- describes: frontend/src/features/settings/installation-configuration.tsx frontend/src/features/settings/configurationentries.ts frontend/src/app/paletteconfiguration.ts backend/internal/compose/handlers_configuration.go backend/internal/compose/configseam.go backend/internal/platform/config/overview.go -->
# Configuration

The **Configuration** page lists every setting the installation reads, what its value is and where that value comes from. It is read-only. It sits in the **Governance** group of Settings, and only users who may change installation settings see it, which Admin and Ops do.

### What does the Configuration page show?
Each row shows the **Key** of the setting with the name that sets it under it, its **Value** and its **Source**. Open a row to read what the setting does, its default, its type and the command-line flag that sets it.

A long value is cut short in its row. Open the row to read it whole, with a button that copies it. A long default shows whole there too, with its own copy button.

### How do I see only the values somebody set?
Press **Not default** above the list. It shows every setting whose value was set outside the app or saved in the app. The other buttons show one source each: **Set outside the app**, **Saved in the app** and **Default value**. **All** shows every row again. Each button shows how many rows it holds.

**All** alone shows a setting that only the background worker or the setup program reads. The app cannot see where its value comes from.

Type in the filter box to narrow the list by key or by the name under it. The buttons then count only the rows that match. If nothing matches, the page says so.

You can also search for a setting by its name, such as `capture.auto_enrich_daily_cap` or `MARGINCE_CAPTURE_AUTO_ENRICH_DAILY_CAP`. Use the settings search or the command palette (⌘K or Ctrl+K). The search offers the **Configuration** page, and the page opens with the filter set to what you typed.

### Where does a value come from?
The **Source** column says which of these set the value:
- **Default value**: nothing set it, so the built-in default applies.
- **Saved in the app**: an administrator saved it in Settings.
- **Command line, Environment, .env file or Configuration file**: whoever runs the installation set it outside the app. The column names the flag, the name or the key, and the file when there is one.

### How do I change a setting set outside the app?
Change it where its source says. A setting set outside the app is locked in the app, and this page does not edit it. The same name and place show beside the locked control on its own Settings page, as [Why can't I change a setting?](settings.md#why-cant-i-change-a-setting) shows.

### Why does a row show "Secret", "Not set" or "No value"?
A secret shows the word "Secret" and never its value or its default. Some settings are read only by the background worker or by the program that sets up the data store. Such a setting shows "Not set", because the app cannot see the sources of those programs. It does not mean the setting is empty. "No value" is a setting the app reads that has neither a value nor a default. "Empty" is a value that is set to nothing.

For the other settings pages, see [Settings](settings.md).
