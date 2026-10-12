{ nixoriumPackage, useKVM ? true }:
{
  name = "nixorium-assistant";
  requiredFeatures.kvm = useKVM;
  nodes.controller = { pkgs, ... }: {
    networking.hostName = "pc99";
    virtualisation.memorySize = 1024;
    users.groups.nixorium-operations = {};
    users.users.admin = {
      isNormalUser = true;
      extraGroups = [ "nixorium-operations" ];
    };
    environment.systemPackages = [ nixoriumPackage pkgs.git pkgs.nix pkgs.python3 pkgs.util-linux ];
    nix.settings.experimental-features = [ "nix-command" "flakes" ];
    systemd.tmpfiles.rules = [
      "d /var/lib/nixorium/assistant 0700 admin users -"
      "d /var/lib/nixorium/coordination 0770 root nixorium-operations -"
      "f /var/lib/nixorium/coordination/operation.lock 0660 root nixorium-operations -"
    ];
    environment.etc."nixorium/assistant.json" = {
      mode = "0444";
      text = builtins.toJSON {
        schemaVersion = 1;
        enabled = true;
        endpoint = "http://127.0.0.1:18480/v1";
        subscribeUrl = "https://example.invalid/assistant/";
      };
    };
    systemd.services.assistant-fake = {
      wantedBy = [ "multi-user.target" ];
      serviceConfig.ExecStart = "${pkgs.python3}/bin/python3 ${./assistant-fake-service.py}";
    };
    environment.etc."assistant-test/settings.json".source = ../templates/site/lab-settings.json;
    environment.etc."assistant-test/flake.nix".text = ''
      {
        outputs = { self }: let
          host = name: file: {
            config.system.build.toplevel.drvPath = "/nix/store/" +
              builtins.hashString "sha256" (name + builtins.toJSON (import file)) + ".drv";
          };
        in {
          nixoriumHostClasses = [
            { representative = "pc01"; members = [ "pc01" ]; }
            { representative = "pc99"; members = [ "pc99" ]; }
          ];
          nixosConfigurations = {
            pc99 = host "pc99" ./modules/controller.nix;
            pc01 = host "pc01" ./modules/course.nix;
          };
          labMeta = {
            schemaVersion = 2;
            version = "vm-test";
            controller = { name = "pc99"; number = 99; staticIp = "10.0.0.99"; dhcpIp = "192.0.2.10"; };
            clients = { count = 1; hosts = [ { name = "pc01"; ip = "10.0.0.1"; } ]; };
            network = { base = "10.0.0.0"; prefixLength = 24; ifaceName = "eth0"; cachePort = 5000; pxeHttpPort = 8080; };
            users = { student = "student"; teacher = "teacher"; };
          };
          deploymentStatus = { ready = true; issues = []; controller = { ready = true; issues = []; requiresKeys = true; }; };
        };
      }
    '';
    environment.etc."assistant-test/terminal.py".text = ''
      import json
      import pathlib
      import re
      import subprocess
      import sys
      import time

      scenario = sys.argv[1]
      log = pathlib.Path("/tmp/assistant-terminal-" + scenario + ".log")
      process = subprocess.Popen(
          ["script", "-qefc", "su - admin -c 'stty rows 40 cols 120; nixorium --repo /home/admin/deployment'", str(log)],
          stdin=subprocess.PIPE, stdout=subprocess.DEVNULL, stderr=subprocess.STDOUT,
          env={**__import__("os").environ, "TERM": "xterm"},
      )
      def output():
          text = log.read_text(errors="replace") if log.exists() else ""
          return re.sub(r"\x1b\[[0-?]*[ -/]*[@-~]", "", text).replace("\r", "")

      def wait(text, after=0):
          deadline = time.monotonic() + 90
          while time.monotonic() < deadline:
              # Terminal wrapping must not change the asserted wording.
              if " ".join(text.split()) in " ".join(output()[after:].split()):
                  return
              if process.poll() is not None:
                  break
              time.sleep(0.1)
          raise AssertionError("Missing " + repr(text) + " in terminal:\n" + output())

      def send(keys):
          process.stdin.write(keys.encode())
          process.stdin.flush()

      try:
          if scenario == "enabled":
              wait("Enter acknowledges")
              send("\r")
              wait("Assistant")
              send("m")
              # First access opens the chat itself: no gate, no service call.
              wait("Not activated")
              wait("Try asking, for example:")
              billing_start = len(output())
              send("/billing\r")
              wait("/billing becomes available after subscribing.", after=billing_start)
              send("/unknown\r")
              wait("Unknown command. Type / to see the commands.")
              assert not pathlib.Path("/tmp/fake-requests.log").exists() or not any(json.loads(line)["method"] == "POST" for line in pathlib.Path("/tmp/fake-requests.log").read_text().splitlines())
              # An unknown command stays in the input; Ctrl+N clears it.
              send("\x0e")
              time.sleep(0.3)
              # Sending asks to activate and keeps the request until then.
              send("Prepare the course for pc01.\r")
              wait("Activate Assistant to send your request.")
              wait("Start a free trial · 10 requests, no payment")
              # Offer loading updates only the suffix in a differential redraw.
              wait("Subscribe")
              wait("360 € + VAT per lab / year")
              assert not any(json.loads(line)["method"] == "POST" for line in pathlib.Path("/tmp/fake-requests.log").read_text().splitlines())
              send("\r")
              wait("https://example.invalid/trial")
              wait("the free trial page in your browser")
              assert re.search(r"\x1b\]8;[^;]*;https://example.invalid/trial(?:\x07|\x1b\\)", log.read_text())
              # Enter opens the page when a desktop is present, then reveals the input.
              send("\r")
              wait("Activation code")
              wait("Paste the code you receive by email.")
              send("TEST-CODE-0001\r")
              wait("Before you send")
              requests = [json.loads(line) for line in pathlib.Path("/tmp/fake-requests.log").read_text().splitlines()]
              assert [r["path"] for r in requests if r["method"] == "POST"] == ["/v1/activate"]
              assert not any(r["userText"] for r in requests)
              # Accepting consent sends the waiting request exactly once.
              send("\r")
              wait("Proposal ready · 1 file changed")
              wait("Ctrl+R to review")
              wait("✓ Evaluation passed · changes pc01")
              # Reopen the unsaved proposal without admitting another turn.
              send("\x0e")
              time.sleep(0.3)
              resume_start = len(output())
              send("/resume\r")
              wait("Enter open · Esc back", after=resume_start)
              wait("proposal not saved", after=resume_start)
              send("\r")
              time.sleep(0.3)
              send("\x12")
              wait("Review the proposal")
              wait("✓ Checked by Nixorium: the configuration is valid")
              wait("Saving creates a local commit and a backup.")
              # Choose "Save only": the stub deployment cannot build the controller.
              send("\x1b[B")
              time.sleep(0.3)
              send("\r")
              wait("Saved in a local commit. Backup:")
              wait("Computers were not updated.")
              wait("try one computer first")
              send("\x1b")
              time.sleep(0.3)
              wait("Saved. Your next message starts a new chat on the saved configuration.")
              after_saved = len(output())
              # Simulate an account acquiring a Stripe customer; no external call.
              # The next turn refreshes the account, which then reports a paid plan.
              pathlib.Path("/tmp/fake-billing-customer").touch()
              send("Confirm the saved course.\r")
              wait("New chat", after=after_saved)
              wait("Saved configuration confirmed.", after=after_saved)
              wait("https://example.invalid/help", after=after_saved)
              # The answer is visible before the turn ends. Keys sent while the
              # turn is still running are discarded, so a partly dropped
              # "/billing" would be submitted as a message. The name tip is
              # drawn only once the chat is idle again.
              wait("Tip: type /name to give this laboratory a name.", after=output().rindex("Saved configuration confirmed."))
              raw = log.read_text()
              assert re.search(r"\x1b\]8;[^;]*;https://example.invalid/help(?:\x07|\x1b\\)", raw)
              assert not any(sequence in raw for sequence in ["\x1b[?1000h", "\x1b[?1002h", "\x1b[?1003h", "\x1b[?1006h"])
              assert "The deployment changed since this chat started" not in output()[after_saved:]
              billing_start = len(output())
              send("/billing\r")
              wait("This link works for a few minutes.", after=billing_start)
              wait("Official electronic invoices are delivered through SDI.", after=billing_start)
              assert re.search(r"\x1b\]8;[^;]*;https://billing\.stripe\.com/p/session\?secret=test_synthetic_vm(?:\x07|\x1b\\)", log.read_text())
              resume_start = len(output())
              send("/resume\r")
              wait("Enter open · Esc back", after=resume_start)
              wait("Prepare the course for pc01.", after=resume_start)
              # The current summary-only conversation is excluded.
              assert "Confirm the saved course." not in output()[resume_start:]
              send("\r")
              wait("This conversation is based on an older configuration.", after=resume_start)
              wait("Continue in a new chat", after=resume_start)
              send("\x1b[B\r")
              time.sleep(0.3)
              reopen_start = len(output())
              send("\r")
              wait("This conversation is based on an older configuration.", after=reopen_start)
              continue_start = len(output())
              send("\r")
              wait('Continuing a previous chat: "Prepare the course for pc01.".', after=continue_start)
              wait("Last answer: Prepared the course for pc01.", after=continue_start)
              # The draft must not be submitted: the existing two turns remain.
              requests = [json.loads(line) for line in pathlib.Path("/tmp/fake-requests.log").read_text().splitlines()]
              assert len([r for r in requests if r["method"] == "POST" and r["path"] == "/v1/turns"]) == 2
              states = [json.loads(p.read_text()) for p in pathlib.Path("/var/lib/nixorium/assistant/conversations").glob("*/meta.json")]
              saved = next(c for c in states if c.get("savedAt"))
              assert saved["title"] == "Prepare the course for pc01."
              assert saved["lastAnswer"] == "Prepared the course for pc01."
              confirmation = next(c for c in states if c.get("title") == "Confirm the saved course.")
              assert confirmation["lastAnswer"] == "Saved configuration confirmed. Visit https://example.invalid/help."
              transcript = json.loads((pathlib.Path("/var/lib/nixorium/assistant/conversations") / confirmation["id"] / "transcript.json").read_text())
              assert transcript[-1]["content"] == confirmation["lastAnswer"]
              assert all(c.get("lastActivityAt") for c in states)
              fresh = next(c for c in states if not c.get("title"))
              assert fresh["base"] == subprocess.check_output(["su", "-", "admin", "-c", "git -C /home/admin/deployment rev-parse HEAD"], text=True).strip()
              assert json.loads((pathlib.Path("/var/lib/nixorium/assistant/conversations") / fresh["id"] / "transcript.json").read_text()) == []
              send("\x1b")
              time.sleep(0.3)
              send("q")
          else:
              # The breadcrumb also appears during startup; wait for the menu actions.
              wait("Refresh local state")
              assert "Assistant" not in output()
              send("q")
          assert process.wait(timeout=15) == 0
      except BaseException:
          print(output())
          raise
      finally:
          if process.poll() is None:
              process.terminate()
              process.wait(timeout=10)
    '';
    system.stateVersion = "26.05";
  };
  testScript = ''
    import json
    start_all()
    controller.wait_for_unit("multi-user.target")
    controller.wait_for_unit("assistant-fake.service")
    controller.wait_for_open_port(18480)
    controller.succeed("install -d -m 0700 -o admin -g users /home/admin/deployment")
    controller.succeed("cp /etc/assistant-test/flake.nix /home/admin/deployment/flake.nix; mkdir /home/admin/deployment/modules; printf '{ course = 1; }\\n' > /home/admin/deployment/modules/course.nix; printf '{}\\n' > /home/admin/deployment/modules/controller.nix")
    settings = json.loads(controller.succeed("cat /etc/assistant-test/settings.json"))
    settings["lab"].update(masterDhcpIp="192.0.2.10", pcCount=1, ifaceName="eth0", adminPassword="$6$vm$not-the-public-default", teacherPassword="$6$vm$not-the-public-default", studentPassword="$6$vm$not-the-public-default")
    controller.succeed("printf %s " + __import__("shlex").quote(json.dumps(settings)) + " > /home/admin/deployment/lab-settings.json; chown -R admin:users /home/admin/deployment")
    controller.succeed("su - admin -c 'cd deployment; git init -q; git add .; git -c user.name=Test -c user.email=test@example.invalid commit -qm initial'")
    original = controller.succeed("su - admin -c 'git -C deployment rev-parse HEAD'").strip()
    with subtest("activation, consent, tools, review and local save"):
      controller.succeed("python3 /etc/assistant-test/terminal.py enabled", timeout=240)
      assert controller.succeed("su - admin -c 'git -C deployment log -1 --format=%s'").strip() == "Save reviewed Assistant proposal"
      assert controller.succeed("su - admin -c 'git -C deployment rev-parse HEAD'").strip() != original
      assert controller.succeed("su - admin -c 'git -C deployment for-each-ref --format=\"%(objectname)\" refs/nixorium/assistant-backups/'").strip() == original
      assert controller.succeed("su - admin -c 'git -C deployment status --porcelain'").strip() == ""
      assert controller.succeed("cat /home/admin/deployment/modules/course.nix").strip() == "{ course = 2; }"
      requests = [json.loads(line) for line in controller.succeed("cat /tmp/fake-requests.log").splitlines()]
      step = "/v1/turns/00000000-0000-4000-8000-000000000001/steps"
      assert [r["path"] for r in requests if r["method"] == "POST"] == ["/v1/activate", "/v1/turns"] + [step] * 3 + ["/v1/turns", step, "/v1/billing-portal"]
      assert [r["userText"] for r in requests if r["method"] == "POST"] == [False, False, True, True, True, False, True, False]
      assert all(not r["userText"] for r in requests if r["method"] == "GET")
      assert controller.succeed("systemctl list-units 'nixorium-apply*' --state=active --no-legend --no-pager").strip() == ""
    with subtest("Assistant is absent without the controller option"):
      controller.succeed("rm /etc/nixorium/assistant.json; python3 /etc/assistant-test/terminal.py disabled", timeout=120)
  '';
}
