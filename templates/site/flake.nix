{
  description = "Private Nixorium deployment";

  inputs.nixorium.url = "github:giovantenne/nixorium/master";

  outputs = { self, nixorium }:
    let
      labConfig = nixorium.lib.evalLabSettings
        (builtins.fromJSON (builtins.readFile ./lab-settings.json));
      labSoftware = builtins.fromJSON (builtins.readFile ./lab-software.json);
      clientGroups = {
        # graphics = [ "pc01" "pc02" ];
      };
      mkDeployment = candidateLabConfig: candidateLabSoftware: nixorium.lib.mkLab {
        deploymentSelf = self;
        labConfig = candidateLabConfig;
        labSoftware = candidateLabSoftware;
        inherit clientGroups;

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
      deployment = mkDeployment labConfig labSoftware;
      validateCandidate = rawSettings:
        let
          candidate = mkDeployment (nixorium.lib.evalLabSettings rawSettings) labSoftware;
          controllerName = candidate.labMeta.controller.name;
        in
        builtins.deepSeq [
          candidate.labMeta
          candidate.deploymentStatus
          candidate.nixosConfigurations.${controllerName}.config.system.build.toplevel.drvPath
        ] true;
      validateSoftwareCandidate = rawSoftware:
        let
          candidate = mkDeployment labConfig rawSoftware;
          clientNames = map (client: client.name) candidate.labMeta.clients.hosts;
        in
        builtins.deepSeq [
          candidate.nixoriumSoftware
          (map
            (name: candidate.nixosConfigurations.${name}.config.system.build.toplevel.drvPath)
            clientNames)
        ] true;
    in
    deployment // {
      # Machine-facing validation hook used before lab-settings.json is written.
      nixoriumValidateCandidate = validateCandidate;
      # Machine-facing validation hook used before lab-software.json is written.
      nixoriumValidateSoftwareCandidate = validateSoftwareCandidate;
    };
}
