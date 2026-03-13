{
  description = "NixOS system configuration";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-25.11";
    disko = {
      url = "github:nix-community/disko";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };

  outputs = { self, nixpkgs, disko }:
    let
      # ── Import lab configuration ─────────────────────────────────
      # Edit lab-config.nix to customize for your environment.
      config = import ./lab-config.nix;

      inherit (config) masterDhcpIp;
      inherit (config) networkBase;
      inherit (config) pcCount;
      inherit (config) masterHostNumber;
      inherit (config) ifaceName;
      inherit (config) teacherUser;
      inherit (config) studentUser;
      inherit (config) teacherPassword;
      inherit (config) studentPassword;
      inherit (config) adminPassword;
      inherit (config) adminSshKey;
      inherit (config) homepageUrl;
      inherit (config) studentGitName;
      inherit (config) studentGitEmail;
      inherit (config) adminGitName;
      inherit (config) adminGitEmail;
      inherit (config) veyonLocationName;
      inherit (config) timeZone;
      inherit (config) defaultLocale;
      inherit (config) extraLocale;
      inherit (config) keyboardLayout;
      inherit (config) consoleKeyMap;

      masterHostName = "pc${toString masterHostNumber}";
      masterIp = "${networkBase}.${toString masterHostNumber}";

      system = "x86_64-linux";
      pcNumbers = builtins.genList (n: n + 1) pcCount;
      clientNumbers = pcNumbers;
      padNumber = n: if n < 10 then "0${toString n}" else toString n;

      # Overlay: packages not available in nixpkgs or needing patches
      labOverlay = final: prev: {
        veyon = final.callPackage ./pkgs/veyon.nix {};
        # gnome-remote-desktop with VNC enabled + multi-session patch
        gnome-remote-desktop = import ./pkgs/gnome-remote-desktop.nix { inherit prev; };
      };

      hostModules = [
        { nixpkgs.overlays = [ labOverlay ]; }
        disko.nixosModules.disko
        ./disko-uefi.nix
        ./modules/hardware.nix
        ./modules/common.nix
        ./modules/users.nix
        ./modules/networking.nix
        ./modules/cache.nix
        ./modules/filesystems.nix
        ./modules/home-reset.nix
        ./modules/veyon.nix
      ];

      labSettings = {
        inherit masterIp;
        inherit masterDhcpIp;
        inherit masterHostName;
        inherit masterHostNumber;
        inherit networkBase;
        inherit pcCount;
        inherit ifaceName;
        inherit teacherUser;
        inherit studentUser;
        inherit teacherPassword;
        inherit studentPassword;
        inherit adminPassword;
        inherit adminSshKey;
        inherit homepageUrl;
        inherit studentGitName;
        inherit studentGitEmail;
        inherit adminGitName;
        inherit adminGitEmail;
        inherit veyonLocationName;
        inherit timeZone;
        inherit defaultLocale;
        inherit extraLocale;
        inherit keyboardLayout;
        inherit consoleKeyMap;
        cachePublicKey = "lab-cache-key:jJsA9nDLNlyzhBOj5rfSKcEL2IwNspxrbNCyqmvdUvI=";
        cachePort = 5000;
      };
      mkHost = n:
        let
          name = "pc${padNumber n}";
          hostIp = "${networkBase}.${toString n}";
        in
        {
          inherit name;
          value = nixpkgs.lib.nixosSystem {
            inherit system;
            specialArgs = {
              inherit labSettings;
              inherit hostIp;
              hostName = name;
            };
            modules = hostModules;
          };
        };
      mkColmenaHost = n:
        let
          name = "pc${padNumber n}";
          hostIp = "${networkBase}.${toString n}";
          address = hostIp;
        in
        {
          inherit name;
          value = {
            _module.args = {
              inherit labSettings;
              inherit hostIp;
              hostName = name;
            };
            imports = hostModules;
            deployment = {
              targetHost = address;
              tags = [ "lab" ];
            };
          };
        };
    in
    assert masterHostNumber > pcCount
      || throw "masterHostNumber (${toString masterHostNumber}) must be greater than pcCount (${toString pcCount})";
    {
      nixosConfigurations = builtins.listToAttrs (map mkHost pcNumbers) // {
        ${masterHostName} = nixpkgs.lib.nixosSystem {
          inherit system;
          specialArgs = {
            inherit labSettings;
            hostIp = "${networkBase}.${toString masterHostNumber}";
            hostName = masterHostName;
          };
          modules = hostModules;
        };
        netboot = nixpkgs.lib.nixosSystem {
          inherit system;
          specialArgs = { inherit labSettings; };
          modules = [
            "${nixpkgs}/nixos/modules/installer/netboot/netboot-minimal.nix"
            ./modules/cache.nix
            ({ pkgs, lib, ... }: {
              # During netboot the master is only reachable on its DHCP address
              nix.settings.substituters = lib.mkForce [ "http://${masterDhcpIp}:${toString labSettings.cachePort}" ];
              networking.useDHCP = lib.mkForce true;
              services.openssh.enable = true;
              environment.systemPackages = [ disko.packages.${system}.default ];
              system.stateVersion = "25.11";
              system.activationScripts.copyFlakeToRamdisk.text = ''
                install -d -m 0755 /installer/repo
                cp -a ${self}/. /installer/repo/
              '';
            })
          ];
        };
      };

      colmena = {
        meta = {
          nixpkgs = import nixpkgs {
            inherit system;
            overlays = [ labOverlay ];
          };
          specialArgs = { inherit labSettings; };
        };
        defaults = {
          deployment = {
            targetUser = "root";
            buildOnTarget = false;
          };
        };
        # Controller deploys to itself locally
        ${masterHostName} = {
          _module.args = {
            inherit labSettings;
            hostIp = "${networkBase}.${toString masterHostNumber}";
            hostName = masterHostName;
          };
          imports = hostModules;
          deployment = {
            targetHost = "localhost";
            tags = [ "master" ];
          };
        };
      } // builtins.listToAttrs (map mkColmenaHost clientNumbers);
    };
}
