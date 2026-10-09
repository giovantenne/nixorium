{ pkgs, useKVM ? true }:
let
  nixoriumPackage = pkgs.callPackage ../pkgs/nixorium.nix {};
  python = pkgs.python3.withPackages (packages: [ packages.pexpect ]);
in
pkgs.testers.runNixOSTest {
  name = "nixorium-git-backup";
  requiredFeatures.kvm = useKVM;
  nodes.controller = { ... }: {
    environment.systemPackages = [ nixoriumPackage pkgs.git pkgs.nix pkgs.openssh pkgs.jq python ];
    services.openssh.enable = true;
    services.openssh.settings.PermitRootLogin = "prohibit-password";
    environment.etc."backup-settings.json".source = ../templates/site/lab-settings.json;
    environment.etc."evaluation-fixture.nix".text = ''
      { outputs = { self }: {
        credentials = (builtins.fromJSON (builtins.readFile ./lab-settings.json)).lab;
        sourceRevision = builtins.fromJSON (builtins.readFile ./.nixorium-source-revision.json);
        offlineCredentials = builtins.toFile "offline-credentials.json"
          (builtins.toJSON (builtins.fromJSON (builtins.readFile ./lab-settings.json)).lab);
      }; }
    '';
    environment.etc."git-backup-terminal.py".source = ./git-backup-terminal.py;
    virtualisation.memorySize = 2048;
    virtualisation.cores = 2;
    system.stateVersion = "26.05";
  };
  testScript = ''
    start_all()
    controller.wait_for_unit("sshd.service")
    controller.succeed("install -d -m 0700 /tmp/lab /tmp/lab/keys /root/.ssh; cp /etc/backup-settings.json /tmp/lab/lab-settings.json; cp /etc/evaluation-fixture.nix /tmp/lab/flake.nix; printf '{\"nodes\":{\"root\":{}},\"root\":\"root\",\"version\":7}' > /tmp/lab/flake.lock; printf 'secret-key\\nadmin-ssh\\nlab-credentials.json\\n' > /tmp/lab/.gitignore")
    controller.succeed("nix --extra-experimental-features nix-command key generate-secret --key-name test-cache > /tmp/lab/secret-key; chmod 0600 /tmp/lab/secret-key; nix --extra-experimental-features nix-command key convert-secret-to-public < /tmp/lab/secret-key > /tmp/lab/keys/cache-public-key; ssh-keygen -q -t ed25519 -N \"\" -f /tmp/lab/admin-ssh; mv /tmp/lab/admin-ssh.pub /tmp/lab/keys/admin-ssh.pub")
    controller.succeed("jq '.lab.credentialsVersion = 1' /tmp/lab/lab-settings.json > /tmp/settings; mv /tmp/settings /tmp/lab/lab-settings.json; jq -n --arg hash '$6$fixture$not-a-real-password' '{version:1, admin:$hash, teacher:$hash, student:$hash}' > /tmp/lab/lab-credentials.json; chmod 0600 /tmp/lab/lab-credentials.json")
    controller.succeed("git -C /tmp/lab init -q; git -C /tmp/lab add .; git -C /tmp/lab -c user.name=Test -c user.email=test@example.invalid commit -qm fixture; git init --bare /tmp/backup.git")
    controller.succeed("ssh-keygen -q -t ed25519 -N \"\" -f /root/.ssh/id_ed25519; cp /root/.ssh/id_ed25519.pub /root/.ssh/authorized_keys; chmod 0600 /root/.ssh/authorized_keys; printf '127.0.0.1 %s\\n' \"$(cat /etc/ssh/ssh_host_ed25519_key.pub)\" > /root/.ssh/known_hosts")
    controller.succeed("install -d -m 0700 /root/.ssh/nixorium-known-hosts; printf 'pc01 trusted-computer-key\\n' > /root/.ssh/nixorium-known-hosts/known_hosts; chmod 0600 /root/.ssh/nixorium-known-hosts/known_hosts; printf 'correct horse battery\\n' >/tmp/passphrase; chmod 0600 /tmp/passphrase")
    with subtest("unbacked keys do not block ordinary configuration checks"):
      controller.fail("nixorium deploy plan --repo /tmp/lab --on @lab --json > /tmp/blocked.json")
      controller.succeed("jq -e 'all(.issues[]; .field != \"backup\") and any(.issues[]; .field == \"configuration\")' /tmp/blocked.json")
    with subtest("reviewed SSH push and exact remote verification"):
      controller.succeed("nixorium backup plan --repo /tmp/lab --remote root@127.0.0.1:/tmp/backup.git --branch main --json > /tmp/plan.json; token=$(jq -r .reviewToken /tmp/plan.json); nixorium backup publish --repo /tmp/lab --remote root@127.0.0.1:/tmp/backup.git --branch main --expect $token --yes --passphrase-file /tmp/passphrase --json > /tmp/published.json || { cat /tmp/published.json; false; }; jq -e '.state == \"completed\"' /tmp/published.json")
      controller.succeed("test \"$(git --git-dir=/tmp/backup.git rev-parse main)\" = \"$(git -C /tmp/lab rev-parse HEAD)\"; test -z \"$(git -C /tmp/lab status --porcelain)\"; ! git --git-dir=/tmp/backup.git ls-tree -r --name-only main | grep -xE 'secret-key|admin-ssh|lab-credentials.json'")
      controller.fail("nixorium backup publish --repo /tmp/lab --remote root@127.0.0.1:/tmp/backup.git --expect stale --yes --passphrase-file /tmp/passphrase")
    with subtest("restore from a fresh home keeps original keys and trust"):
      controller.succeed("install -d -m 0700 /tmp/replacement-home /tmp/replacement-home/.ssh /tmp/replacement-home/.ssh/nixorium-known-hosts; touch /tmp/replacement-home/.ssh/nixorium-known-hosts/known_hosts; chmod 0600 /tmp/replacement-home/.ssh/nixorium-known-hosts/known_hosts; HOME=/tmp/replacement-home nixorium backup clone --remote root@127.0.0.1:/tmp/backup.git --to /tmp/restored-lab --yes --passphrase-file /tmp/passphrase --json > /tmp/restored.json || { cat /tmp/restored.json; false; }; jq -e '.state == \"completed\"' /tmp/restored.json")
      controller.succeed("for key in secret-key admin-ssh lab-credentials.json; do cmp /tmp/lab/$key /tmp/restored-lab/$key; test $(stat -c '%a' /tmp/restored-lab/$key) = 600; done; cmp /root/.ssh/nixorium-known-hosts/known_hosts /tmp/replacement-home/.ssh/nixorium-known-hosts/known_hosts; test -z \"$(git -C /tmp/restored-lab status --porcelain)\"")
      controller.fail("nixorium backup clone --remote root@127.0.0.1:/tmp/backup.git --to /tmp/restored-lab --yes --passphrase-file /tmp/passphrase")
      controller.succeed("printf 'wrong passphrase\\n' > /tmp/wrong; chmod 0600 /tmp/wrong")
      controller.fail("nixorium backup clone --remote root@127.0.0.1:/tmp/backup.git --to /tmp/wrong-restore --yes --passphrase-file /tmp/wrong")
      controller.succeed("test ! -e /tmp/wrong-restore")
    with subtest("local Nix source injects hashes and preserves the original revision"):
      controller.succeed("nixorium config source --repo /tmp/restored-lab --revision $(git -C /tmp/restored-lab rev-parse HEAD) > /tmp/source; source=$(cat /tmp/source); nix --extra-experimental-features 'nix-command flakes' eval --json $source#credentials > /tmp/evaluated; jq -e --arg hash '$6$fixture$not-a-real-password' '.adminPassword == $hash and .teacherPassword == $hash and .studentPassword == $hash' /tmp/evaluated")
      controller.succeed("source=$(cat /tmp/source); test $(nix --extra-experimental-features 'nix-command flakes' eval --raw $source#sourceRevision) = $(git -C /tmp/restored-lab rev-parse HEAD); directory=$(printf %s $source | cut -c6-); test ! -e $directory/secret-key; test ! -e $directory/admin-ssh; test ! -e $directory/lab-credentials.json; ! grep -F '$6$' /tmp/restored-lab/lab-settings.json; test -z \"$(git -C /tmp/restored-lab status --porcelain)\"")
      controller.succeed("source=$(cat /tmp/source); offline=$(nix --extra-experimental-features 'nix-command flakes' eval --raw $source#offlineCredentials); cmp /tmp/evaluated $offline || jq -e --slurpfile expected /tmp/evaluated '. == $expected[0]' $offline")
    with subtest("an ordinary clone exposes no account hashes"):
      controller.succeed("git clone --branch main /tmp/backup.git /tmp/model-clone; test ! -e /tmp/model-clone/lab-credentials.json; ! git -C /tmp/model-clone grep -F '$6$' $(git -C /tmp/model-clone rev-list --all)")
    with subtest("standalone restore screen over a real terminal"):
      controller.succeed("python /etc/git-backup-terminal.py", timeout=60)
  '';
}
