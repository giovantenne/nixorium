{ config, lib, hostName, labSettings, ... }:
{
  options.services.nixorium.assistant = {
    endpoint = lib.mkOption {
      type = lib.types.str;
      default = "https://assistant.nixorium.org/v1";
      description = "Assistant service endpoint (override only for staging or tests)";
    };
    subscribeUrl = lib.mkOption {
      type = lib.types.str;
      default = "https://nixorium.org/pricing/";
      description = "Assistant subscription page (override only for staging or tests)";
    };
  };

  config = lib.mkIf (hostName == labSettings.masterHostName && (labSettings.assistant or true)) {
    environment.etc."nixorium/assistant.json" = {
      mode = "0444";
      text = builtins.toJSON {
        schemaVersion = 1;
        enabled = true;
        inherit (config.services.nixorium.assistant) endpoint;
        inherit (config.services.nixorium.assistant) subscribeUrl;
      };
    };
    systemd.tmpfiles.rules = [ "d /var/lib/nixorium/assistant 0700 admin users -" ];
  };
}
