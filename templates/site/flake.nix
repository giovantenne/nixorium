{
  description = "Private Nixorium deployment";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";
  inputs.nixorium.url = "github:giovantenne/nixorium/v2.0.0-beta.4";
  inputs.nixorium.inputs.nixpkgs.follows = "nixpkgs";

  outputs = { self, nixpkgs, nixorium }:
    let
      packageBase = nixorium.lib.packageBase;
      packageBaseCompatible =
        packageBase.source == "github:NixOS/nixpkgs"
        && packageBase.channel == "nixos-26.05";
      # deploymentMode is optional: existing deployments remain laboratories.
      # Explicit controller mode requires pcCount = 0 (see README).
      labConfig = nixorium.lib.evalLabSettings
        (builtins.fromJSON (builtins.readFile ./lab-settings.json));
      labSoftware = builtins.fromJSON (builtins.readFile ./lab-software.json);
      softwareCatalog = import ./software-catalog.nix;
      clientGroups = {
        # graphics = [ "pc01" "pc02" ];
      };
      mkDeployment = candidateLabConfig: candidateLabSoftware:
        assert packageBaseCompatible
          || throw "The configured nixpkgs pin is not compatible with this Nixorium release";
        nixorium.lib.mkLab {
          deploymentSelf = self;
          labConfig = candidateLabConfig;
          labSoftware = candidateLabSoftware;
          inherit softwareCatalog;
          inherit clientGroups;
          homeResetEphemeralPaths = [
            ".local/share/docker"
            ".local/npm"
            ".npm"
          ];

          publicKeys = {
            cache = ./keys/cache-public-key;
            ssh = ./keys/admin-ssh.pub;
            veyon = ./keys/veyon-public-key.pem;
          };

          assets = {
            logo = ./assets/logo.txt;
            backgrounds = [
              ./assets/backgrounds/1-ristretto.jpg
              ./assets/backgrounds/2-ristretto.jpg
              ./assets/backgrounds/3-ristretto.jpg
            ];
            mimeApps = ./assets/mimeapps.list;
            vscodeSettings = ./assets/vscode-settings.json;
          };

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
          representativeClient =
            if candidate.labMeta.clients.hosts == [] then null
            else (builtins.head candidate.labMeta.clients.hosts).name;
          representativeClientSystem =
            if representativeClient == null then []
            else [
              candidate.nixosConfigurations.${representativeClient}.config.system.build.toplevel.drvPath
            ];
        in
        # Every generated client shares the managed-software module graph.
        # Validate its declarations and one representative system instead of
        # forcing the complete laboratory inventory for each plan and save.
        builtins.deepSeq ([ candidate.nixoriumSoftware ] ++ representativeClientSystem) true;
    in
    deployment // {
      nixoriumPackageBase = {
        schemaVersion = 1;
        inherit (packageBase) source channel;
        revision = nixpkgs.rev or "";
      };
      # Machine-facing validation hook used before lab-settings.json is written.
      nixoriumValidateCandidate = validateCandidate;
      # Client validation before saving software. The management application
      # also uses mkLab's controller validation hook for shared/controller scopes.
      nixoriumValidateSoftwareCandidate = validateSoftwareCandidate;
    };
}
