{
  description = "Private Nixorium deployment";

  inputs.nixorium.url = "github:giovantenne/nixorium/v2.0.0-beta.3";

  outputs = { self, nixorium }:
    nixorium.lib.mkLab {
      deploymentSelf = self;
      labConfig = import ./lab-config.nix;

      publicKeys = {
        cache = ./keys/cache-public-key;
        ssh = ./keys/admin-ssh.pub;
        veyon = ./keys/veyon-public-key.pem;
      };

      assets.logo = ./assets/logo.txt;

      sharedModules = [ ./modules/shared.nix ];
      controllerModules = [ ./modules/controller.nix ];
      clientModules = [ ./modules/clients.nix ];

      hostModules = {
        # pc05 = [ ./modules/pc05.nix ];
      };
    };
}
