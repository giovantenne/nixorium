{ nixoriumPackage, useKVM ? true }:
{
  name = "nixorium-host-diagnostics";
  requiredFeatures.kvm = useKVM;
  nodes.machine = { pkgs, ... }: {
    virtualisation.memorySize = 768;
    users.users.admin.isNormalUser = true;
    environment.systemPackages = [ nixoriumPackage ];
    environment.etc."nixorium-host-diagnostics.json".text = builtins.toJSON {
      controller = true;
      systemctl = "${pkgs.systemd}/bin/systemctl";
      journalctl = "${pkgs.systemd}/bin/journalctl";
    };
    system.configurationRevision = "0123456789012345678901234567890123456789";
    # A disposable failure exercises real systemd manager/unit journal records.
    systemd.services.home-reset = {
      wantedBy = [ "multi-user.target" ];
      serviceConfig.Type = "oneshot";
      script = ''
        echo "No space left on device /home/student/private.txt"
        echo "password=synthetic-secret student@example.org"
        exit 1
      '';
    };
    system.stateVersion = "26.05";
  };
  testScript = ''
    import json
    machine.wait_for_unit("multi-user.target")
    with subtest("fixed immutable configuration and bounded useful logs"):
      assert machine.succeed("readlink -f /etc/nixorium-host-diagnostics.json").strip().startswith("/nix/store/")
      # Keep command failures visible in this synthetic fixture; production
      # reports intentionally contain no arbitrary command diagnostics.
      boot = machine.succeed("cat /proc/sys/kernel/random/boot_id").strip()
      machine.succeed("journalctl --boot=" + boot.replace("-", "") + " --lines=50 --no-pager --output=json --output-fields=__REALTIME_TIMESTAMP,PRIORITY,_SYSTEMD_UNIT,UNIT,MESSAGE --unit=home-reset.service > /tmp/journal-fixture.json")
      data = machine.succeed("nixorium-host-diagnostics home_reset")
      report = json.loads(data)
      assert len(data.encode()) < 32768 and report["schemaVersion"] == 1
      assert report["bootId"] == machine.succeed("cat /proc/sys/kernel/random/boot_id").strip()
      assert report["revision"] == "0123456789012345678901234567890123456789", report
      reset = next(unit for unit in report["units"] if unit["name"] == "home-reset.service")
      assert reset["activeState"] == "failed" and reset["result"] == "exit-code" and reset["changedAt"], report
      assert "journal" not in report["unavailable"] and 0 < len(report["journal"]) <= 50, report
      assert any("No space left on device" in entry["excerpt"] for entry in report["journal"]), report
      assert any("Failed to start" in entry["excerpt"] for entry in report["journal"]), report
      assert not any(secret in data for secret in ["synthetic-secret", "student@example.org", "private.txt"]), data
      assert report["space"] and all(item["totalBytes"] >= item["availableBytes"] for item in report["space"])
    with subtest("restricted local journal access is explicitly unavailable"):
      report = json.loads(machine.succeed("su - admin -c 'nixorium-host-diagnostics home_reset'"))
      assert "journal" in report["unavailable"], report
      assert report["journal"] == [], report
    with subtest("no arbitrary unit, path or command and no reset mutation"):
      machine.fail("nixorium-host-diagnostics 'system;touch /tmp/diagnostics-mutated'")
      machine.fail("nixorium-host-diagnostics system extra")
      machine.fail("test -e /tmp/diagnostics-mutated")
      assert machine.succeed("systemctl show home-reset.service --property=NRestarts --value").strip() == "0"
      machine.fail("test -e /var/lib/home-snapshots/.workspace-reset/pending.json")
  '';
}
