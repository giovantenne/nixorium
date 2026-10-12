"""Exercise first-run consent through the packaged TUI in a real terminal."""
import json
import os
from pathlib import Path

import pexpect


def launch():
    return pexpect.spawn(
        "nixorium", ["--repo", "/tmp/onboarding-lab"],
        env={**os.environ, "TERM": "xterm-256color"},
        dimensions=(30, 120), encoding="utf-8", timeout=30,
    )


def quit_at_menu(child):
    child.expect("Laboratory overview")
    child.send("q")
    child.expect(pexpect.EOF)
    child.close()
    assert child.exitstatus == 0


# Quitting the disclaimer must not record acknowledgement or consent.
child = launch()
child.expect("Accept & continue")
child.send("q")
child.expect(pexpect.EOF)
assert not list(Path("/root/.local/state/nixorium/dashboard").glob("*-disclaimer.json"))

# Accept the disclaimer, inspect the invitation, then skip to the menu.
child = launch()
child.expect("Accept & continue")
child.send("\r")
child.expect("Off — no adoption reports are being sent")
child.send("\x1b")
quit_at_menu(child)
assert not Path("/var/lib/nixorium/telemetry/state.json").exists()
acknowledgements = list(Path("/root/.local/state/nixorium/dashboard").glob("*-disclaimer.json"))
assert len(acknowledgements) == 1
assert json.loads(acknowledgements[0].read_text())["version"] == 1

# The next invocation must invite again. A refusal must survive another launch.
child = launch()
child.expect("Off — no adoption reports are being sent")
assert "Accept & continue" not in child.before
child.send("n")
quit_at_menu(child)
state = json.loads(Path("/var/lib/nixorium/telemetry/state.json").read_text())
assert state["consent"] == "disabled" and "secret" not in state
child = launch()
child.expect("Laboratory overview")
assert "Share statistics" not in child.before
child.send("c")
child.expect("Client computers have not been configured yet")
child.send("\x1b")
quit_at_menu(child)
assert json.loads(Path("/var/lib/nixorium/telemetry/state.json").read_text()) == state
