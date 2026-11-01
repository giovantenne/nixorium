{ nixoriumPackage, useKVM ? true }:
{
  name = "nixorium-host-diagnostics";
  requiredFeatures.kvm = useKVM;
  nodes.machine = { pkgs, ... }: {
    virtualisation.memorySize = 768;
    users.users.admin.isNormalUser = true;
    # A USB tablet and a virtio display give the hardware area real devices.
    virtualisation.qemu.options = [ "-device qemu-xhci" "-device usb-tablet" "-device virtio-gpu-pci" "-device intel-hda" "-device hda-duplex" ];
    boot.initrd.availableKernelModules = [ "virtio_gpu" "xhci_pci" "usbhid" ];
    boot.kernelModules = [ "snd_hda_intel" ];
    services.printing.enable = true;
    environment.systemPackages = [ nixoriumPackage ];
    environment.etc."nixorium-host-diagnostics.json".text = builtins.toJSON {
      controller = true;
      systemctl = "${pkgs.systemd}/bin/systemctl";
      journalctl = "${pkgs.systemd}/bin/journalctl";
      lpstat = "${pkgs.cups}/bin/lpstat";
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
    with subtest("hardware area reports devices without serial numbers"):
      data = machine.succeed("nixorium-host-diagnostics hardware")
      report = json.loads(data)
      print(json.dumps(report, indent=1))
      assert len(data.encode()) < 32768 and report["area"] == "hardware", report
      assert any(device["vendorId"] == "0627" and device["productId"] == "0001" for device in report["usbDevices"]), report["usbDevices"]
      assert all(set(device) <= {"path", "vendorId", "productId", "manufacturer", "product", "class", "interfaceClasses"} for device in report["usbDevices"])
      assert report["pciDevices"] and all(device["vendorId"] and device["deviceId"] for device in report["pciDevices"]), report
      assert report["displays"] and all(display["connector"].startswith("card") for display in report["displays"]), report
      assert report["audioCards"] and report["audioCards"][0]["driver"], report
      assert "printers" not in report["unavailable"] and "usb" not in report["unavailable"] and "displays" not in report["unavailable"], report
      assert all(entry["unit"] == "kernel" for entry in report["journal"]) and len(report["journal"]) <= 50, report
      assert "serial" not in data.lower(), data
      machine.fail("nixorium-host-diagnostics hardware extra")
    with subtest("restricted local journal access is explicitly unavailable"):
      report = json.loads(machine.succeed("su - admin -c 'nixorium-host-diagnostics home_reset'"))
      assert "journal" in report["unavailable"], report
      assert report["journal"] == [], report
    with subtest("no arbitrary unit, path or command and no reset mutation"):
      machine.fail("nixorium-host-diagnostics 'system;touch /tmp/diagnostics-mutated'")
      machine.fail("nixorium-host-diagnostics system extra")
      # Allowlisted system files (ADR 0032): read, list, refuse secrets and homes.
      hosts = json.loads(machine.succeed("nixorium-host-diagnostics file /etc/hosts"))
      assert hosts["kind"] == "file" and "localhost" in hosts["content"] and hosts["resolved"].startswith("/nix/store/"), hosts
      listing = json.loads(machine.succeed("nixorium-host-diagnostics file /etc"))
      names = [entry["name"] for entry in listing["entries"]]
      assert "hosts" in names and "shadow" not in names and listing["hidden"] >= 1, listing
      for refused in ["/etc/shadow", "/etc/ssh/ssh_host_ed25519_key", "/root", "/home", "/etc/../root/.ssh"]:
        machine.fail("nixorium-host-diagnostics file " + refused)
      # Links into the system's own locations, as on real hardware.
      address = json.loads(machine.succeed("nixorium-host-diagnostics file /sys/class/net/lo/address"))
      assert address["content"].strip() == "00:00:00:00:00:00" and address["resolved"].startswith("/sys/devices/"), address
      model = json.loads(machine.succeed("nixorium-host-diagnostics file /sys/class/dmi/id/product_name"))
      assert model["kind"] == "file" and model["resolved"].startswith("/sys/devices/"), model
      mounts = json.loads(machine.succeed("nixorium-host-diagnostics file /proc/mounts"))
      assert " / " in mounts["content"], mounts
      for refused in ["/sys/class/dmi/id/product_serial", "/sys/class/dmi/id/product_uuid", "/etc/machine-id", "/etc/static/sudoers.d"]:
        machine.fail("nixorium-host-diagnostics file " + refused)
      machine.fail("nixorium-host-diagnostics file '/etc/hosts;touch /tmp/file-mutated'")
      machine.fail("test -e /tmp/file-mutated")
      machine.fail("test -e /tmp/diagnostics-mutated")
      assert machine.succeed("systemctl show home-reset.service --property=NRestarts --value").strip() == "0"
      machine.fail("test -e /var/lib/home-snapshots/.workspace-reset/pending.json")
  '';
}
