{ lib, ... }:
{
  # UpdateHostKeys would let OpenSSH rewrite ~/.ssh/known_hosts, replacing
  # the controller's link to the Nixorium-managed copy with a stale file: a
  # reviewed reinstalled computer then kept reading as "SSH key changed".
  programs.ssh.extraConfig = ''
    Host pc*
      StrictHostKeyChecking accept-new

    Host *
      UpdateHostKeys no
  '';

  services.openssh = {
    enable = true;
    settings.PasswordAuthentication = false;
  };

  environment.sessionVariables = {
    GIT_ASKPASS = "";
    SSH_ASKPASS_REQUIRE = "never";
  };
  environment.variables.SSH_ASKPASS = lib.mkForce "";
}
