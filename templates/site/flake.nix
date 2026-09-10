{
  description = "Private Nixorium deployment";

  inputs.nixorium.url = "github:giovantenne/nixorium/master";

  outputs = { self, nixorium }:
    let
      labConfig = nixorium.lib.evalLabSettings
        (builtins.fromJSON (builtins.readFile ./lab-settings.json));
      mkDeployment = candidateLabConfig: nixorium.lib.mkLab {
        deploymentSelf = self;
        labConfig = candidateLabConfig;

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
      deployment = mkDeployment labConfig;
      validateCandidate = rawSettings:
        let
          candidate = mkDeployment (nixorium.lib.evalLabSettings rawSettings);
          controllerName = candidate.labMeta.controller.name;
        in
        builtins.deepSeq [
          candidate.labMeta
          candidate.deploymentStatus
          candidate.nixosConfigurations.${controllerName}.config.system.build.toplevel.drvPath
        ] true;
    in
    deployment // {
      # Machine-facing validation hook used before lab-settings.json is written.
      nixoriumValidateCandidate = validateCandidate;
    };
}
