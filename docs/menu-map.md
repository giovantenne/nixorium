Generated from the TUI by scripts/generate-docs.sh; do not edit it by hand.
It is for tools and documentation that need exact menu paths.

## Administrator menu

Computers (c): Inventory, updating computers, Internet access and power
Computers → Classroom view (v): See, control and lock the students' screens, show yours, send your desktop
Computers → Computer inventory (h): Check reachability and compare observed systems with the intended revision
Computers → Update computers (d): Make the selected client computers run the saved configuration
Computers → Queued updates (u): Computers that update when they are switched on
Computers → Power controls (x): Switch on, shut down or restart selected client computers after review
Computers → Internet access (i): Temporarily block or restore Internet on selected clients
Computers → Lock screens (l): Lock or unlock the screens of selected students' computers
Computers → Send files (s): Copy a file or folder to the desktops of selected students' computers
Installation (n): Configure the lab and install computers by PXE or the official USB ISO over SSH
Installation → Network boot (PXE) (p): Prepare, start or finish network installation; recover interrupted networking here
Installation → USB over SSH (u): Install one physically identified computer from the official NixOS ISO
Software (w): Manage configured software or find packages to add
Maintenance (m): Settings, controller updates, services, revisions, logs and diagnostics
Maintenance → Change settings (e): Network, account names and passwords, regional values, browser, Git and controller keys
Maintenance → Change settings → Network (n): Controller/client interfaces, address, and laboratory subnet
Maintenance → Change settings → Computers (c): Client count and controller host number
Maintenance → Change settings → Accounts (a): Account names and passwords
Maintenance → Change settings → Accounts → Change account names (n): Teacher and student user names
Maintenance → Change settings → Accounts → Change account passwords (p): Administrator, teacher and student passwords
Maintenance → Change settings → Accounts → Change account passwords → Administrator (a): Controller administrator account
Maintenance → Change settings → Accounts → Change account passwords → Teacher (t): Classroom teacher account
Maintenance → Change settings → Accounts → Change account passwords → Student (s): Classroom student account
Maintenance → Change settings → Regional (r): Time zone and keyboard layout
Maintenance → Change settings → Browser (b): Default classroom homepage
Maintenance → Change settings → Classroom view (v): Turn the teacher's classroom view on or off
Maintenance → Change settings → Git (g): Student and administrator commit identity
Maintenance → Change settings → Student workspace (w): Desktop, dock, editor extensions and initial student preferences
Maintenance → Diagnostics (i): Check the lab and see recovery instructions
Maintenance → Update system and packages (b): Refresh the NixOS base or review a channel migration
Maintenance → Update Nixorium (u): Choose master or a release fetched from the configured upstream
Maintenance → View operation logs (l): Recent outcomes and bounded deployment log tails
Maintenance → Back up lab (y): Save an encrypted file or keep a private Git repository up to date
Maintenance → Restore lab (o): Restore a backup file or Git backup into a new folder with the original keys
Maintenance → Free disk space (f): Remove old system versions on the controller and clients, after review
Maintenance → Apply to controller (c): Make this controller run the saved configuration, after review
Maintenance → Change network addressing (n): Move the computers and the controller to a new laboratory network, step by step
Maintenance → Controller services (s): Check software delivery services or restart the signed cache when troubleshooting
Maintenance → Review Git changes (g): Inspect and commit selected safe deployment files
Maintenance → Reset deployment template (t): Replace local customizations from the locked upstream; preserve settings and keys with a backup
Maintenance → Adoption statistics (a): Preview optional daily reports, enable or disable sharing
Assistant (a): Describe a lab customization; review the proposal before saving

## Teacher dashboard

Computers → Classroom view (v): See, control and lock the students' screens, show yours, send your desktop
Computers → Computer inventory (h): Check reachability and compare observed systems with the intended revision
Computers → Power controls (x): Switch on, shut down or restart selected client computers after review
Computers → Internet access (i): Temporarily block or restore Internet on selected clients
Computers → Lock screens (l): Lock or unlock the screens of selected students' computers
Computers → Send files (s): Copy a file or folder to the desktops of selected students' computers
