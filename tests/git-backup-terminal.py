import io
from pathlib import Path

import pexpect

# No deployment is present in this working directory. F1 cannot restore, and
# cancelling does not create a folder or expose the masked passphrase.
child = pexpect.spawn("nixorium", ["backup", "clone"], cwd="/tmp", env={"PATH": "/run/current-system/sw/bin", "TERM": "xterm"}, dimensions=(30, 120), encoding="utf-8", timeout=20)
transcript = io.StringIO()
child.logfile_read = transcript
child.expect("Restore lab")
child.send("git@github.com:school/private-lab.git\r")
child.send("\r")
child.send("/tmp/tui-restored\r")
child.send("quiet recovery password")
child.send("\x1bOP")
child.expect("close help")
child.send("\r")
child.send("\x1b")
child.expect("SSH repository")
child.send("\x1b")
child.expect(pexpect.EOF)
assert "quiet recovery password" not in transcript.getvalue()
assert not Path("/tmp/tui-restored").exists()
child.close()
assert child.exitstatus == 0
