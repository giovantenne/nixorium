{ lib }:
rawConfig:
let
  ipv4Octets = value:
    let
      matched = builtins.match "([0-9]+)\\.([0-9]+)\\.([0-9]+)\\.([0-9]+)" value;
    in
    if matched == null then null else map lib.toInt matched;
  isIpv4 = value:
    let
      octets = ipv4Octets value;
    in
    octets != null && builtins.all (octet: octet >= 0 && octet <= 255) octets;
  ipv4ToInt = value:
    let
      octets = ipv4Octets value;
    in
    builtins.elemAt octets 0 * 16777216
      + builtins.elemAt octets 1 * 65536
      + builtins.elemAt octets 2 * 256
      + builtins.elemAt octets 3;
  pow2 = exponent:
    if exponent == 0 then 1 else 2 * pow2 (exponent - 1);
  isUserName = value:
    builtins.match "[a-z_][a-z0-9_-]{0,30}" value != null;
  isPasswordHash = value:
    builtins.match "\\$6\\$[^$]+\\$[^$]+" value != null;
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
            description = "IPv4 network address of the static lab network";
          };
          networkPrefixLength = lib.mkOption {
            type = lib.types.ints.between 1 30;
            description = "CIDR prefix length of the static lab network";
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
  networkSize = pow2 (32 - config.networkPrefixLength);
in
assert config.masterHostNumber > config.pcCount
  || throw "masterHostNumber (${toString config.masterHostNumber}) must be greater than pcCount (${toString config.pcCount})";
assert config.masterDhcpIp == "MASTER_DHCP_IP" || isIpv4 config.masterDhcpIp
  || throw "masterDhcpIp must be an IPv4 address or the template placeholder MASTER_DHCP_IP";
assert isIpv4 config.networkBase
  || throw "networkBase must be an IPv4 network address such as 10.0.0.0";
assert lib.mod (ipv4ToInt config.networkBase) networkSize == 0
  || throw "networkBase (${config.networkBase}) is not aligned to /${toString config.networkPrefixLength}";
assert config.masterHostNumber < networkSize - 1
  || throw "masterHostNumber (${toString config.masterHostNumber}) does not fit in ${config.networkBase}/${toString config.networkPrefixLength}";
assert builtins.match "[A-Za-z0-9][A-Za-z0-9_.:-]{0,14}" config.ifaceName != null
  || throw "ifaceName must be a valid Linux interface name of at most 15 characters";
assert isUserName config.teacherUser
  || throw "teacherUser must be a valid Unix user name";
assert isUserName config.studentUser
  || throw "studentUser must be a valid Unix user name";
assert config.teacherUser != config.studentUser
  || throw "teacherUser and studentUser must be different";
assert !builtins.elem config.teacherUser [ "root" "admin" ]
  || throw "teacherUser must not be root or admin";
assert !builtins.elem config.studentUser [ "root" "admin" ]
  || throw "studentUser must not be root or admin";
assert isPasswordHash config.teacherPassword
  || throw "teacherPassword must be a SHA-512 crypt hash beginning with $6$";
assert isPasswordHash config.studentPassword
  || throw "studentPassword must be a SHA-512 crypt hash beginning with $6$";
assert isPasswordHash config.adminPassword
  || throw "adminPassword must be a SHA-512 crypt hash beginning with $6$";
assert builtins.match "https?://.+" config.homepageUrl != null
  || throw "homepageUrl must use http:// or https://";
config
