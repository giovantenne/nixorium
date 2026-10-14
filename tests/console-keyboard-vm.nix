{ pkgs, useKVM ? true }:
# The text console follows the XKB layout (console.useXkbConfig), the way
# modules/desktop.nix configures installed machines. With keyboardLayout "it"
# a password with AltGr symbols, set as an installer-style SHA-512 hash, must be
# accepted at the console login when typed on the Italian keys. The keys are
# sent as physical QEMU key names, so the login only succeeds if the console
# keymap really is Italian. Limitation: GDM is not started here, so the graphical
# side is covered by the Nix evaluation checks (services.xserver.xkb.layout) and
# not typed through a GNOME login screen.
let
  # Password: Ab@#[]ò{}~€7  (installer-style hash, fixed salt)
  passwordHash = "$6$nixoriumsalt0001$QzkoY3mws1xp80v6/L3ZYQ9ILQ/eMbiJ5mU6ggC5LulcMrC5ycPDuEM1Bh1ysKj77eYeUQS/o5xIph0Pi3B4R0";
in
pkgs.testers.runNixOSTest {
  name = "nixorium-console-keyboard";
  requiredFeatures.kvm = useKVM;
  nodes.machine = { ... }: {
    services.xserver.xkb.layout = "it";
    console.useXkbConfig = true;
    users.mutableUsers = false;
    users.users.owner = {
      isNormalUser = true;
      hashedPassword = passwordHash;
    };
    system.stateVersion = "26.05";
  };
  testScript = ''
    machine.wait_for_unit("multi-user.target")
    with subtest("the console keymap is derived from the XKB layout"):
      machine.succeed("grep -q '^KEYMAP=' /etc/vconsole.conf")
      machine.succeed("! grep -q '^KEYMAP=us$' /etc/vconsole.conf")
      machine.succeed("systemctl is-active systemd-vconsole-setup.service")
      machine.succeed("cat /etc/vconsole.conf >&2")

    with subtest("a symbol password typed on the Italian keys logs in"):
      machine.wait_until_tty_matches("1", "login: ")
      machine.send_chars("owner\n")
      machine.wait_until_tty_matches("1", "Password: ")
      for key in [
          "shift-a", "b",
          "alt_r-semicolon",     # @  (AltGr + o grave)
          "alt_r-apostrophe",    # #  (AltGr + a grave)
          "alt_r-bracket_left",   # [  (AltGr + e grave)
          "alt_r-bracket_right",  # ]  (AltGr + plus)
          "semicolon",           # o grave
          "alt_r-7",             # {
          "alt_r-0",             # }
          "alt_r-equal",         # ~  (AltGr + i grave)
          "alt_r-e",             # euro
          "7",
          "ret",
      ]:
        machine.send_key(key)
      machine.send_chars("touch loggedin\n")
      machine.wait_until_succeeds("test -e /home/owner/loggedin", timeout=60)
    with subtest("the live installer keymap (kbd it plus overlay) types the same password"):
      machine.succeed("rm /home/owner/loggedin")
      machine.succeed("chvt 2")
      machine.wait_until_tty_matches("2", "login: ")
      # install.sh loads kbd "it" and then this AltGr overlay; start from us so
      # only the installer keymap can make the password work.
      machine.succeed("loadkeys us")
      machine.succeed("printf '%s\\n' 'altgr keycode 8 = braceleft' 'altgr keycode 9 = bracketleft' 'altgr keycode 10 = bracketright' 'altgr keycode 11 = braceright' 'altgr keycode 13 = asciitilde' 'altgr keycode 18 = U+20AC' | loadkeys it -")
      machine.send_chars("owner\n")
      machine.wait_until_tty_matches("2", "Password: ")
      for key in [
          "shift-a", "b", "alt_r-semicolon", "alt_r-apostrophe",
          "alt_r-bracket_left", "alt_r-bracket_right", "semicolon",
          "alt_r-7", "alt_r-0", "alt_r-equal", "alt_r-e", "7", "ret",
      ]:
        machine.send_key(key)
      machine.send_chars("touch loggedin\n")
      machine.wait_until_succeeds("test -e /home/owner/loggedin", timeout=60)
  '';
}
