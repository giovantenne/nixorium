package presentation

const assistantHelpText = `How Assistant works
  Describe what your lab should do. Assistant prepares the change in a private
  copy and checks it. Nothing is saved or installed until you review and save it.
  Each message you send uses one request from your trial or yearly allowance.

Commands
  /review   Review the current proposal and save it.
  /save     Same as /review: opens the review to save the proposal.
  /new      Start a new chat. Use it when you change topic or after saving.
  /resume   Open a previous conversation. Opening it does not use a request.
  /billing   Manage your subscription and see payments (opens Stripe).
  /report   Tell us one answer was helpful or wrong. Sends this conversation
            to Nixorium once, so we can fix mistakes. Kept for 90 days.
  /share    Turn automatic sharing on or off (off by default). When on, every
            conversation of this lab is sent to Nixorium to improve Assistant.
            Kept for 90 days.
  /code     Enter an activation code (for example after subscribing).
  /help     Show this help.

Keys
  Enter        Send the message
  Alt+Enter    New line (Ctrl+J also works)
  Ctrl+R       Review the proposal
  Ctrl+N       New chat
  PgUp/PgDn    Scroll the conversation
  Select text      Drag with the mouse (or Shift+drag in some terminals)
  Esc          Stop Assistant while it works, or go back`

var assistantCommandDescriptions = map[string]string{
	"/review":  "Review the current proposal and save it.",
	"/save":    "Same as /review: opens the review to save the proposal.",
	"/new":     "Start a new chat.",
	"/resume":  "Open a previous conversation. Opening it does not use a request.",
	"/billing": "Manage your subscription and see payments (opens Stripe).",
	"/report":  "Tell us one answer was helpful or wrong.",
	"/share":   "Turn automatic sharing on or off (off by default).",
	"/code":    "Enter an activation code (for example after subscribing).",
	"/help":    "Show this help.",
}
