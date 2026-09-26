{ nixoriumPackage, useKVM ? true }:
let
  management = import ./management-vm.nix { inherit nixoriumPackage useKVM; };
in
management // {
  name = "nixorium-pxe-legacy-lock";
  # Reuse the real management units and their sandbox. Stop before any Nix
  # build by deliberately leaving the configured deployment absent.
  testScript = ''
    start_all()
    controller.wait_for_unit("multi-user.target")
    controller.wait_for_unit("nixorium-test-network.service")
    controller.succeed("test ! -e /home/admin/nixorium-deployment")
    controller.succeed("systemctl show nixorium-prepare-pxe.service -p ProtectHome --value | grep -Fx read-only")
    controller.succeed("systemctl show nixorium-prepare-pxe.service -p CapabilityBoundingSet --value | grep -E '^$'")

    def prepare_failure(message):
      controller.fail("systemctl start nixorium-prepare-pxe.service")
      invocation = controller.succeed("systemctl show nixorium-prepare-pxe.service -p InvocationID --value").strip()
      log = controller.succeed("journalctl --no-pager -o cat _SYSTEMD_INVOCATION_ID=" + invocation)
      assert message in log, log
      assert "Read-only file system" not in log, log
      controller.succeed("systemctl reset-failed nixorium-prepare-pxe.service")

    with subtest("missing and idle legacy locks permit preparation through the read-only sandbox"):
      prepare_failure("configured deployment path is not a real directory")
      controller.succeed("su - admin -c 'mkdir -p ~/.local/state/nixorium/operations; printf legacy-lock > ~/.local/state/nixorium/operations/deploy.lock; chmod 600 ~/.local/state/nixorium/operations/deploy.lock'")
      prepare_failure("configured deployment path is not a real directory")
      controller.succeed("test $(cat /home/admin/.local/state/nixorium/operations/deploy.lock) = legacy-lock; test $(stat -c '%U:%G:%a' /home/admin/.local/state/nixorium/operations/deploy.lock) = admin:users:600")

    with subtest("an active old process still prevents overlapping preparation"):
      controller.succeed("systemd-run --unit=nixorium-test-legacy-lock --property=User=admin sh -ec 'exec 8<>/home/admin/.local/state/nixorium/operations/deploy.lock; /run/current-system/sw/bin/flock -n 8; : > /tmp/nixorium-legacy-lock-held; exec /run/current-system/sw/bin/sleep infinity'")
      controller.wait_until_succeeds("test -f /tmp/nixorium-legacy-lock-held", timeout=30)
      prepare_failure("a legacy Nixorium deployment is still running")
      controller.succeed("systemctl stop nixorium-test-legacy-lock.service")
      prepare_failure("configured deployment path is not a real directory")

    with subtest("unsafe permissions and symlinks remain rejected"):
      controller.succeed("chmod 644 /home/admin/.local/state/nixorium/operations/deploy.lock")
      prepare_failure("legacy deployment lock is unsafe")
      controller.succeed("chmod 600 /home/admin/.local/state/nixorium/operations/deploy.lock; mv /home/admin/.local/state/nixorium/operations/deploy.lock /home/admin/.local/state/nixorium/operations/legacy-target; su - admin -c 'ln -s legacy-target ~/.local/state/nixorium/operations/deploy.lock'")
      prepare_failure("legacy deployment lock is unsafe")
      controller.succeed("rm /home/admin/.local/state/nixorium/operations/deploy.lock; mv /home/admin/.local/state/nixorium/operations/legacy-target /home/admin/.local/state/nixorium/operations/deploy.lock")
      prepare_failure("configured deployment path is not a real directory")
  '';
}
