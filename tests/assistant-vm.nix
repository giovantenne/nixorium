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
              wait("Welcome to Assistant")
              wait("Start a free trial · 5 requests, no payment")
              # Offer loading updates only the suffix in a differential redraw.
              wait("Subscribe")
              wait("360 € + VAT per lab / year")
              assert not any(json.loads(line)["method"] == "POST" for line in pathlib.Path("/tmp/fake-requests.log").read_text().splitlines())
              send("\r")
              wait("https://example.invalid/trial")
              wait("Register with your email, then enter the code you receive.")
              wait("Activation code")
              send("TEST-CODE-0001\r")
              wait("Before you send")
              requests = [json.loads(line) for line in pathlib.Path("/tmp/fake-requests.log").read_text().splitlines()]
              assert [r["path"] for r in requests if r["method"] == "POST"] == ["/v1/activate"]
              assert not any(r["userText"] for r in requests)
              # The same chat footer appeared during activation. Wait for a
              # fresh redraw after consent before typing into the chat.
              after_consent = len(output())
              send("\r")
              wait("Enter send · Alt+Enter new line", after=after_consent)
              assert not any(json.loads(line)["path"] == "/v1/turns" for line in pathlib.Path("/tmp/fake-requests.log").read_text().splitlines())
              send("/unknown\r")
              wait("Unknown command. Type / to see the commands.")
              send("\x0e")
              time.sleep(0.3)
              send("\r")
              wait("Try asking, for example:")
              assert not any(json.loads(line)["path"] == "/v1/turns" for line in pathlib.Path("/tmp/fake-requests.log").read_text().splitlines())
              send("\x0e")
              time.sleep(0.3)
              send("Prepare the course for pc01.\r")
              wait("Proposal ready · 1 file changed")
              wait("Ctrl+R to review")
              wait("✓ Evaluation passed · changes pc01")
              send("\x12")
              wait("Review the proposal")
              wait("✓ Checked by Nixorium: the configuration is valid")
              send("\r")
              wait("Save? A local commit and a backup are created. Nothing is installed.")
              send("\r")
              wait("Saved in a local commit. Backup:")
              wait("Nothing was built or activated.")
              wait("Try it on one computer first:")
              send("\x1b")
              time.sleep(0.3)
              wait("Saved. Your next message starts a new chat on the saved configuration.")
              after_saved = len(output())
              send("Confirm the saved course.\r")
              wait("New chat", after=after_saved)
              wait("Saved configuration confirmed.", after=after_saved)
              assert "The deployment changed since this chat started" not in output()[after_saved:]
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
      assert [r["path"] for r in requests if r["method"] == "POST"] == ["/v1/activate", "/v1/turns"] + [step] * 3 + ["/v1/turns", step]
      assert [r["userText"] for r in requests if r["method"] == "POST"] == [False, False, True, True, True, False, True]
      assert all(not r["userText"] for r in requests if r["method"] == "GET")
      assert controller.succeed("systemctl list-units 'nixorium-apply*' --state=active --no-legend --no-pager").strip() == ""
    with subtest("Assistant is absent without the controller option"):
      controller.succeed("rm /etc/nixorium/assistant.json; python3 /etc/assistant-test/terminal.py disabled", timeout=120)
  '';
}
