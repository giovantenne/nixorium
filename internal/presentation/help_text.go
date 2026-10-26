package presentation

// helpText is the static part of one screen's contextual help. head comes
// before the lines that depend on model state, tail after them.
type helpText struct{ head, tail []string }

var shutdownHelp = helpText{head: []string{
	"Space select   a select/deselect all   n select those on at the last check   r check computers   Tab shutdown/restart   Enter check/review", "u acknowledge unknown sessions in review   Esc cancel", "An accepted request does not prove physical power state or a completed restart.",
}}

var logsHelp = helpText{head: []string{
	"Enter open log   r refresh list   ↑/↓/pg scroll detail",
}}

var gitReviewHelp = helpText{head: []string{
	"c select commit paths   x discard changes   u undo the last saved change   Space select   a all safe paths", "r refresh review   ↑/↓/pg scroll patch", "Enter on a commit review creates a local commit; DISCARD (typed) restores files after saving their content; Enter on the undo review saves a new change that puts the files of the last saved change back. Nothing is pushed and no computer changes.", "After an undo, Enter on the result opens Apply to controller; then update the computers.",
}}

var updateHelp = helpText{head: []string{
	"During validation: l expands or collapses current check details in place, when there are details.",
	"The review shows what changes; F4 Details adds every version, the checks and the line-by-line changes to the deployment files, which are also saved in its Git history.",
}}

// screenHelp lists every screen with its own help. A screen present with
// no text still has help: its lines are all built from model state.
var screenHelp = map[dashboardScreen]helpText{
	dashboardManagedJobs: helpText{head: []string{
		"Tab switches between controller and PXE jobs; status refreshes automatically.", "Esc leaves this read-only view; quitting does not cancel a managed job.", "Interrupted means a running progress record has no running unit.", "Inspect the named journal unit, then request a fresh review before retrying.", "Completion here is not verification of the current configuration.",
	}},
	dashboardHostTrust: helpText{head: []string{
		"Compare the offered fingerprint with this computer's physical console.", "Only a deliberately reinstalled client is eligible for reviewed key rotation.", "Esc cancels the read or review without changes; saving cannot be interrupted.",
	}},
	dashboardHome: helpText{},
	dashboardAssistantReview: helpText{head: []string{
		"Choose Technical details to scroll the full comparison.", "Save and update the controller (Ctrl+R, then Enter) saves a local commit and a backup, updates this controller, then opens the computer review with the computers the change alters selected; when it alters none, no computer needs updating.", "Save only keeps the change; update computers later from Computers → Update computers. Esc goes back.",
		"After saving, update one computer first and check it before the others.",
	}},
	dashboardAssistant: helpText{tail: []string{
		"Messages and files leave the controller only after subscription and consent checks.",
	}},
	dashboardComputersArea: helpText{},
	dashboardInstallationArea: helpText{tail: []string{
		"r  Reattach the recorded USB operation when available",
	}},
	dashboardUSBInstall: helpText{head: []string{
		"Tab or arrows move through console fields; the password is masked and cleared after use.", "Esc before apply requests confirmed cleanup. After apply, Esc detaches and never retries Disko.",
		"An unfinished installation says what Nixorium found (does the computer answer, was the disk touched) and what you can do: a resumes it, n checks how far it got, b restarts the installed computer, v checks the installed system.",
		"x cancels safely and c closes the record; both ask only for Enter when the disk was not touched. Closing after the disk was written asks you to type CLOSE.",
		"While the installer job runs the view refreshes by itself with read-only status requests. d shows the raw result fields.",
		"A failed connection's original error remains visible across status refreshes in this TUI session.",
	}},
	dashboardAdministration: helpText{},
	dashboardHosts: helpText{head: []string{
		"r refresh computers   / search names, addresses or status", "Enter open details   t technical detail", "Search owns all text keys until Enter or Esc.",
	}},
	dashboardDeploy: helpText{head: []string{
		"Review probes only selected computers; it does not prove power state or authenticated identity.", "Results list each computer; Shift+arrows scroll long results and l opens private logs.",
	}},
	dashboardBackup: helpText{head: []string{
		"Backup is optional and never blocks installation or updates. Choose an encrypted file or a private Git repository.",
		"An encrypted file contains the entire deployment and Git history. Mount the USB first, choose an existing folder outside the deployment, and enter a passphrase of at least 12 characters. Review, then Enter creates the archive.",
		"For Git, use the SSH URL of a PRIVATE GitHub/GitLab repository. Configure an independent SSH key/agent and verify the host fingerprint outside Nixorium first.",
		"The first Git backup reviews committed files, encrypts recovery keys, then pushes when you press Enter after confirming the repository is private. The remote branch is verified after sending; rejected or uncertain pushes are not backups.",
		"Afterwards saved changes are pushed automatically while Nixorium is open, without the passphrase. Back up now pushes immediately. When keys, account passwords or trusted computers change, the passphrase is asked again.",
		"For Git backups, configuration and Git history remain readable; private keys and managed account password hashes are encrypted. Other ignored files are excluded. Review custom files for credentials before committing.",
		"Keep the passphrase and Git access separately. Changing the passphrase does not re-encrypt older Git history.",
		"Restore lab needs a NEW absolute directory and preserves original keys. Existing trusted keys must match; empty stores are filled. It never activates the restored configuration.",
		"Restore lab accepts a backup file or a Git backup. Without an existing deployment, nixorium backup clone opens the same screen.",
	}},
	dashboardCleanup: helpText{head: []string{
		"Space select   a select all   Enter review   Esc Maintenance",
	}, tail: []string{
		"Type CLEAN on the review to start. Computers that are off are never queued. After the result: n new review.",
	}},
	dashboardShare: helpText{head: []string{
		"Type the path of a file or folder (it starts at your home), Enter; then Space selects clients, a selects all, Enter reviews and Enter sends.", "A folder arrives with its name and contents; hidden files and links inside it are left out; at most 2000 items and 500 MB. Students get their own copy, never executable; existing names are kept as \"name (2)\".", "Computers that are off or have nobody signed in are listed and skipped. A restart empties the student's home.",
	}},
	dashboardNetworkChange: helpText{head: []string{
		"Type the new network as address/prefix, for example 10.1.0.0/24, then Enter reviews it. Every computer keeps its number.",
		"The review checks every computer: c continues without those that are off (they move when they are next updated, or are queued). Enter saves the step; no computer changes yet.",
		"Then apply to the controller (it answers on both networks) and update the computers (each moves to its new address), each with its own review confirmed with Enter.",
		"During the change Enter opens the next step; f finishes when every computer is on the new network; u undoes the change, which is possible until it is finished.",
		"After finishing, prepare network installation again before installing a computer by PXE.",
	}},
	dashboardDeferredUpdates: helpText{head: []string{
		"These computers were off when an update was reviewed. Every minute the controller updates those that answer, together, with the same build, check and record as Update computers.", "A computer marked as changed is not updated: the configuration moved on after queueing. Review the update again.", "x removes the selected computer, X removes all; they keep their current system. r refreshes.",
	}},
	dashboardLock: helpText{head: []string{
		"Space select clients; a select all; Tab choose lock or unlock; Enter review, then Enter applies.", "A locked screen shows \"Eyes on the teacher\" and ignores keyboard and mouse; logging out or restarting unlocks it.", "Computers that are off or have nobody signed in are listed and skipped.",
	}},
	dashboardInternet: helpText{head: []string{
		"Space select clients; a select all; Tab choose block or unblock; Enter review.", "r checks Internet access on all clients (read-only); n selects those the chosen action would change.", "Enter applies the reviewed change. Reboot restores Internet; offline clients are never queued.",
	}},
	dashboardDeployReview: helpText{head: []string{
		"The availability shown is a brief SSH-port check, not authenticated identity or proof of power state.", "A failed or interrupted update may require recovery before another operation.", "F3, when some computers did not answer, chooses whether they update automatically when switched on (Computers → Queued updates) or are skipped.", "F2, when offered, creates a new review for only the reachable computers with an open SSH port.", "The new review requires a fresh DEPLOY confirmation; it never starts automatically.", "Esc during replanning keeps the original review and clears its confirmation.",
	}},
	dashboardShutdown:       shutdownHelp,
	dashboardShutdownReview: shutdownHelp,
	dashboardShutdownResult: shutdownHelp,
	dashboardSetup: helpText{head: []string{
		"Enter continue the observed stage   t full checklist",
	}},
	dashboardSettings: helpText{tail: []string{
		"y  Controller keys", "/  Search categories; Esc clears the search before leaving",
		"Accounts opens a small menu: n changes the teacher and student user names, p changes account passwords.",
		"After saving, Enter opens a separate controller review; after verified application it opens client selection. Esc leaves the result.",
	}},
	dashboardWorkspace: helpText{head: []string{
		"Choose Desktop, Dock or Browser, then a supported field. Initial files opens the shared student-home content editor.",
		"Inherit keeps the deployment baseline; Clear means an explicit empty list.",
		"In a list: Space toggles, i inherits, c clears; Shift arrows reorder favorites.",
		"Students' VS Code settings and extensions are not edited here: ask the Assistant. Existing ones are kept when you save.",
		"Enter keeps a field in the draft; Esc cancels that field edit.",
		"v reviews the complete draft, dependencies and student destinations.",
		"Enter on the review saves and records just the workspace profile locally.",
		"Enter then opens a separate controller review. Runtime opt-in, deployment and home reset are never automatic.",
		"A partial save opens Git review for inspection; do not replay the old save or apply systems before recovery.",
		"Packaged extension updates use Maintenance → Update system and packages, which can also change the editor, desktop and operating system.",
	}},
	dashboardHomeFiles: helpText{head: []string{
		"Initial files are files and folders that every student finds in their home, on the controller and on every client computer.",
		"Each time a computer starts, the student's home goes back to this content. Students can change the files during the session.",
		"a adds a file or a folder from this computer; Tab completes the path. Then choose where it goes inside the student's home.",
		"Nixorium keeps a copy in the lab configuration and on every computer: add teaching materials only, never passwords or keys.",
		"Check the copied list. Files start as not executable; Space marks the selected file as a program.",
		"u replaces the selected file or folder with a new copy; d removes it. Changing the original later does not update the copy.",
		"v reviews the changes; Enter on the review saves them. Then update the computers: students get the files at the next start.",
		"Esc goes back without saving. If a save is only partly done, check the saved configuration before updating the computers.",
	}},
	dashboardTemplateReset: helpText{head: []string{
		"Choose a software preset from the exact framework revision already locked in this deployment.",
		"Review all removed/replaced paths with arrows or Page Up/Down; no file contents are exposed.",
		"Existing custom software, home preferences, assets and modules are replaced, not merged.",
		"Settings, lock, keys, ignore rules and untracked files are preserved; collisions block the reset.",
		"Type RESET DEPLOYMENT and Enter to create a backup and a local reset commit. Esc cancels review.",
		"The guided home is enabled in configuration only. Apply systems and reboot separately; no push.",
		"The result offers a separate controller review, then client selection after verified application. Esc leaves without applying.",
		"An interrupted reset requires recovery; preserve the backup ref and .git/nixorium-template-reset.json.",
	}},
	dashboardSettingsReview: helpText{head: []string{
		"Enter saves the reviewed settings; Esc cancels without changing any file.",
		"Network check: after a change of network device or controller DHCP address, Nixorium reads the controller's devices and asks each computer that answers whether it has the named device. Nothing is changed.",
		"A device the controller does not have blocks the save. A device missing on a computer that answers blocks it until you press c to save anyway for the computers listed; c again undoes that.",
		"Computers that do not answer are listed: check them at their screen. Update one computer first and check it answers before updating the others.",
	}},
	dashboardSettingsAccounts: helpText{head: []string{
		"n  Change account names: edit the teacher and student user names, then review and save.",
		"p  Change account passwords: choose the administrator, teacher or student account.",
		"A new password takes effect only after you apply the configuration to this controller and then update the computers.",
		"Esc returns to Settings.",
	}},
	dashboardSettingsPasswords: helpText{head: []string{
		"a  Administrator", "t  Teacher", "s  Student", "Selecting an account opens protected password input; it does not save changes.",
	}},
	dashboardPXE: helpText{head: []string{
		"Enter next step   p configure/prepare   s review start   x stop   r refresh", "l progress details while preparing, when there are details; q closes only the view",
	}},
	dashboardController: helpText{head: []string{
		"n new review   d result details   l logs / progress detail", "q closes the view; systemd-owned work continues",
	}},
	dashboardRecoveryReview: helpText{},
	dashboardServices: helpText{head: []string{
		"c review cache restart   r refresh   l logs after result",
	}},
	dashboardLogs:            logsHelp,
	dashboardLogDetail:       logsHelp,
	dashboardGitReview:       gitReviewHelp,
	dashboardGitCommitSelect: gitReviewHelp,
	dashboardGitCommitReview: gitReviewHelp,
	dashboardGitUndoReview:   gitReviewHelp,
	dashboardUpdate:          updateHelp,
	dashboardUpdateReview:    updateHelp,
	dashboardDiagnostics: helpText{head: []string{
		"↑/↓ move   Enter technical evidence   r run checks again",
	}},
	dashboardTelemetry: helpText{head: []string{
		"Optional daily adoption statistics. e or Enter on the overview shares statistics; n declines or disables sharing.", "p opens the local exact report; i opens Privacy & retention. Arrow keys scroll; r refreshes local settings.", "Esc returns from details, or skips the first invitation to the menu without deciding. It returns next launch until you accept or decline. q exits. Opening this screen uploads no adoption report.", "After the first invitation, change your choice in Maintenance → Adoption statistics.",
	}},
	dashboardSupport: helpText{head: []string{
		"↑/↓, PgUp/PgDown, Home/End scroll the exact filtered JSON.",
		"Enter saves this preview to a private local file; nothing is uploaded.",
		"Esc cancels collection or leaves without saving. r collects a fresh preview.",
		"Versions, revision, time and counts remain visible. Review before sharing.",
		"Unavailable sections are not healthy results. Detailed logs are not included.",
	}},
	dashboardSoftware: helpText{head: []string{
		"F2 selected   F3 package search   F4 suggestions   Tab next view", "p add a deployment-owned profile   / search",
		"Configured software: Enter opens details; from there, c changes its computers and x reviews removal.", "Profile packages: Space include/exclude   Enter choose computers and review", "A profile adds a group of packages; existing packages keep their destinations.",
		"After saving: Enter takes the displayed next step; Esc leaves it for later. Client deployment always has its own review and confirmation.",
	}},
}
