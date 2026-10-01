{ pkgs }:
# The generation cleanup helper on a real NixOS system: older generations
# beyond the newest ten go, the running/booted one stays, garbage is
# collected, a stale review is refused and the boot menu step runs.
let
  helper = pkgs.writeShellApplication {
    name = "nixorium-clean-generations";
    runtimeInputs = [ pkgs.coreutils pkgs.gnused ];
    text = builtins.readFile ../scripts/clean-generations.sh;
  };
in
pkgs.testers.runNixOSTest {
  name = "nixorium-clean-generations";
  nodes.machine = { ... }: {
    environment.systemPackages = [ helper ];
    # Without a boot loader the boot menu step is a harmless no-op.
    boot.loader.grub.enable = false;
    # Test nodes omit switch-to-configuration; managed hosts always have it.
    system.switch.enable = true;
    system.stateVersion = "26.05";
  };
  testScript = ''
    machine.wait_for_unit("multi-user.target")
    system = machine.succeed("readlink -f /run/current-system").strip()
    # A real generation, twelve distinct fake ones, then the real system again.
    machine.succeed(f"nix-env --profile /nix/var/nix/profiles/system --set {system}")
    for number in range(12):
        machine.succeed(f"mkdir -p /tmp/fake-{number} && echo {number} > /tmp/fake-{number}/marker")
        path = machine.succeed(f"nix-store --add /tmp/fake-{number}").strip()
        machine.succeed(f"nix-env --profile /nix/var/nix/profiles/system --set {path}")
    machine.succeed(f"nix-env --profile /nix/var/nix/profiles/system --set {system}")
    generations = sorted(
        (int(name.split("-")[1]) for name in machine.succeed("ls /nix/var/nix/profiles").split() if name.startswith("system-")),
        reverse=True,
    )
    older = generations[10:]
    real = [n for n in older if machine.succeed(f"readlink -f /nix/var/nix/profiles/system-{n}-link").strip() == system]
    expected = sorted(n for n in older if n not in real)
    assert len(expected) == 3 and real, (generations, real)
    plan = machine.succeed("nixorium-clean-generations --plan")
    print(plan)
    assert plan.count(" newest") == 10, plan
    assert f"keep {real[0]} running" in plan, plan
    removed = sorted(int(line.split()[1]) for line in plan.splitlines() if line.startswith("remove "))
    assert removed == expected, (removed, expected)
    expect = [line.split()[1] for line in plan.splitlines() if line.startswith("expect ")][0]
    oldest = machine.succeed(f"readlink -f /nix/var/nix/profiles/system-{expected[0]}-link").strip()
    machine.fail("nixorium-clean-generations --apply 0000000000000000")
    machine.succeed(f"test -e /nix/var/nix/profiles/system-{expected[0]}-link")
    # The unprivileged plan works; apply requires root.
    machine.succeed("su nobody -s /bin/sh -c '/run/current-system/sw/bin/nixorium-clean-generations --plan'")
    result = machine.succeed(f"nixorium-clean-generations --apply {expect}")
    print(result)
    assert "removed " + " ".join(map(str, expected)) in result, result
    assert "boot-menu ok" in result, result
    machine.fail(f"test -e /nix/var/nix/profiles/system-{expected[0]}-link")
    machine.succeed(f"test -e /nix/var/nix/profiles/system-{real[0]}-link")
    machine.fail(f"test -e {oldest}")
    after = machine.succeed("nixorium-clean-generations --plan")
    assert "remove " not in after, after
    empty = [line.split()[1] for line in after.splitlines() if line.startswith("expect ")][0]
    assert "unchanged" in machine.succeed(f"nixorium-clean-generations --apply {empty}")
  '';
}
