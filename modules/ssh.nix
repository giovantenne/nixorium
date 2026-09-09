{ lib, ... }:
{
  programs.ssh.extraConfig = ''
    Host pc*
      StrictHostKeyChecking accept-new
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
