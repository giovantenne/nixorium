{ lib }:
rawConfig:
let
  evaluated = lib.evalModules {
    modules = [
      {
        options.lab = {
          masterDhcpIp = lib.mkOption {
            type = lib.types.str;
            description = "DHCP address of the controller during PXE installation";
          };
          networkBase = lib.mkOption {
            type = lib.types.str;
            description = "First three octets of the static lab network";
          };
          pcCount = lib.mkOption {
            type = lib.types.ints.between 1 253;
            description = "Number of client PCs";
          };
          masterHostNumber = lib.mkOption {
            type = lib.types.ints.between 1 254;
            description = "Host number of the controller";
          };
          ifaceName = lib.mkOption {
            type = lib.types.str;
            description = "Network interface shared by all lab PCs";
          };
          teacherUser = lib.mkOption {
            type = lib.types.str;
            description = "Teacher account name";
          };
          studentUser = lib.mkOption {
            type = lib.types.str;
            description = "Student account name";
          };
          teacherPassword = lib.mkOption {
            type = lib.types.str;
            description = "SHA-512 password hash for the teacher account";
          };
          studentPassword = lib.mkOption {
            type = lib.types.str;
            description = "SHA-512 password hash for the student account";
          };
          adminPassword = lib.mkOption {
            type = lib.types.str;
            description = "SHA-512 password hash for the admin account";
          };
          homepageUrl = lib.mkOption {
            type = lib.types.str;
            description = "Chromium homepage";
          };
          studentGitName = lib.mkOption {
            type = lib.types.str;
            description = "Git author name for the student home template";
          };
          studentGitEmail = lib.mkOption {
            type = lib.types.str;
            description = "Git author email for the student home template";
          };
          adminGitName = lib.mkOption {
            type = lib.types.str;
            description = "Git author name for the admin home template";
          };
          adminGitEmail = lib.mkOption {
            type = lib.types.str;
            description = "Git author email for the admin home template";
          };
          timeZone = lib.mkOption {
            type = lib.types.str;
            description = "IANA time zone";
          };
          defaultLocale = lib.mkOption {
            type = lib.types.str;
            description = "Default system locale";
          };
          extraLocale = lib.mkOption {
            type = lib.types.str;
            description = "Locale used for regional formats";
          };
          keyboardLayout = lib.mkOption {
            type = lib.types.str;
            description = "XKB keyboard layout";
          };
          consoleKeyMap = lib.mkOption {
            type = lib.types.str;
            description = "Linux console keymap";
          };
          veyonNativeHosts = lib.mkOption {
            type = lib.types.listOf lib.types.str;
            default = [];
            description = "Hosts using Veyon's native PipeWire backend";
          };
        };
      }
      {
        config.lab = rawConfig;
      }
    ];
  };
  config = evaluated.config.lab;
in
assert config.masterHostNumber > config.pcCount
  || throw "masterHostNumber (${toString config.masterHostNumber}) must be greater than pcCount (${toString config.pcCount})";
assert config.masterDhcpIp != ""
  || throw "masterDhcpIp must not be empty";
assert config.networkBase != ""
  || throw "networkBase must not be empty";
assert config.ifaceName != ""
  || throw "ifaceName must not be empty";
config
