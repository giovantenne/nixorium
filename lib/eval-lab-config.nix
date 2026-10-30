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
  isIfaceName = value:
    builtins.match "[A-Za-z0-9][A-Za-z0-9_.:-]{0,14}" value != null;
  isNonEmpty = value:
    builtins.match ".*[^[:space:]].*" value != null;
  # Git identities and the keyboard layout reach shell scripts and
  # configuration files; internal/domain/settings.go holds the same rules.
  # Lengths are in bytes, as in the Go validation.
  isGitName = value:
    builtins.stringLength value <= 256 && builtins.match "[^[:cntrl:]]*" value != null;
  # Emails are not checked for shape: a laboratory may already use any text
  # Git accepts. Quoting keeps them safe; only control characters are refused.
  isGitEmail = isGitName;
  isKeyboardLayout = value:
    builtins.stringLength value <= 128 && builtins.match "[A-Za-z0-9_,()+-]+" value != null;
  isAbsoluteHttpUrl = value:
    builtins.match "https?://[^/[:space:]]+(/.*)?" value != null;
  # RFC 3339 date-time in the form time.RFC3339 produces and parses.
  isRfc3339 = value:
    builtins.match
      "[0-9]{4}-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])T([01][0-9]|2[0-3]):[0-5][0-9]:[0-5][0-9](\\.[0-9]+)?(Z|[+-]([01][0-9]|2[0-3]):[0-5][0-9])"
      value != null;
  evaluated = lib.evalModules {
    modules = [
      {
        options.lab = {
          classroomView = lib.mkOption {
            type = lib.types.bool;
            default = true;
            description = "Classroom view agent on client computers (on unless set to false)";
          };
          assistant = lib.mkOption {
            type = lib.types.bool;
            default = true;
            description = "Assistant menu on the controller (on unless set to false; activation and consent are separate)";
          };
          deploymentMode = lib.mkOption {
            type = lib.types.enum [ "laboratory" "controller" ];
            default = "laboratory";
            description = "Explicit controller-only bootstrap or configured laboratory";
          };
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
            type = lib.types.ints.between 0 253;
            description = "Number of client PCs";
          };
          masterHostNumber = lib.mkOption {
            type = lib.types.ints.between 1 254;
            description = "Host number of the controller";
          };
          ifaceName = lib.mkOption {
            type = lib.types.str;
            description = "Fallback network interface for laboratory hosts";
          };
          controllerIfaceName = lib.mkOption {
            type = lib.types.nullOr lib.types.str;
            default = null;
            description = "Optional controller-specific laboratory interface";
          };
          clientIfaceName = lib.mkOption {
            type = lib.types.nullOr lib.types.str;
            default = null;
            description = "Optional default interface for client hosts";
          };
          hostIfaceNames = lib.mkOption {
            type = lib.types.attrsOf lib.types.str;
            default = {};
            description = "Optional per-host laboratory interface overrides";
          };
          teacherUser = lib.mkOption {
            type = lib.types.str;
            description = "Teacher account name";
          };
          studentUser = lib.mkOption {
            type = lib.types.str;
            description = "Student account name";
          };
          credentialsVersion = lib.mkOption {
            type = lib.types.ints.unsigned;
            default = 0;
            description = "Public version of the local account credentials";
          };
          teacherPassword = lib.mkOption {
            type = lib.types.str;
            default = "$6$t.4PBRDwSMnGbuzA$fLuu1n700q.Mvj0ivauGLPQJcfT6XnFMkDh6T0GMWH/hzlSNuzxfh0bxh2iQR027y7PSdzuIvWoO3NgRbM/gV0";
            description = "SHA-512 password hash for the teacher account";
          };
          studentPassword = lib.mkOption {
            type = lib.types.str;
            default = "$6$t.4PBRDwSMnGbuzA$fLuu1n700q.Mvj0ivauGLPQJcfT6XnFMkDh6T0GMWH/hzlSNuzxfh0bxh2iQR027y7PSdzuIvWoO3NgRbM/gV0";
            description = "SHA-512 password hash for the student account";
          };
          adminPassword = lib.mkOption {
            type = lib.types.str;
            default = "$6$t.4PBRDwSMnGbuzA$fLuu1n700q.Mvj0ivauGLPQJcfT6XnFMkDh6T0GMWH/hzlSNuzxfh0bxh2iQR027y7PSdzuIvWoO3NgRbM/gV0";
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
          networkTransition = lib.mkOption {
            type = lib.types.nullOr (lib.types.submodule {
              options = {
                previousBase = lib.mkOption {
                  type = lib.types.str;
                  description = "IPv4 network address the laboratory is moving from";
                };
                previousPrefixLength = lib.mkOption {
                  type = lib.types.ints.between 1 30;
                  description = "CIDR prefix length of the previous laboratory network";
                };
                startedAt = lib.mkOption {
                  type = lib.types.str;
                  description = "RFC 3339 time at which the change of laboratory network started";
                };
              };
            });
            default = null;
            description = ''
              Change of laboratory network in progress, managed by the guided
              change. While it is set the controller also keeps its previous
              address and clients also trust the previous controller address.
              Ignored when no client computers are configured.
            '';
          };
          consoleKeyMap = lib.mkOption {
            type = lib.types.nullOr lib.types.str;
            default = null;
            description = ''
              Deprecated and ignored. The console follows keyboardLayout
              through console.useXkbConfig; the key is accepted so existing
              lab-settings.json files keep evaluating.
            '';
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
  padNumber = number: if number < 10 then "0${toString number}" else toString number;
  validHostNames = [ "pc${padNumber config.masterHostNumber}" ]
    ++ builtins.genList (index: "pc${padNumber (index + 1)}") config.pcCount;
  unknownInterfaceHosts = builtins.filter
    (name: !(builtins.elem name validHostNames))
    (builtins.attrNames config.hostIfaceNames);
  requiredNonEmptyFields = [
    "studentGitName"
    "studentGitEmail"
    "adminGitName"
    "adminGitEmail"
    "timeZone"
    "defaultLocale"
    "extraLocale"
    "keyboardLayout"
  ];
  emptyRequiredFields = builtins.filter
    (name: !isNonEmpty config.${name})
    requiredNonEmptyFields;
  # A change of laboratory network concerns client computers only; without
  # them the record is ignored (mkLab reports it as a warning).
  transition = config.networkTransition;
  checkTransition = transition != null && config.pcCount > 0;
  previousNetworkSize = pow2 (32 - transition.previousPrefixLength);
  # Two aligned networks overlap when the larger one contains the other's base.
  sharedNetworkSize = pow2 (32 - lib.min transition.previousPrefixLength config.networkPrefixLength);
  transitionOverlaps =
    builtins.div (ipv4ToInt transition.previousBase) sharedNetworkSize
      == builtins.div (ipv4ToInt config.networkBase) sharedNetworkSize;
in
assert (config.deploymentMode == "controller" && config.pcCount == 0)
  || (config.deploymentMode == "laboratory" && config.pcCount > 0)
  || throw "pcCount must be zero in controller mode and positive in laboratory mode";
assert config.masterHostNumber > config.pcCount
  || throw "masterHostNumber (${toString config.masterHostNumber}) must be greater than pcCount (${toString config.pcCount})";
assert config.masterDhcpIp == "MASTER_DHCP_IP" || isIpv4 config.masterDhcpIp
  || throw "masterDhcpIp must be an IPv4 address or the template placeholder MASTER_DHCP_IP";
assert isIpv4 config.networkBase
  || throw "networkBase must be an IPv4 network address such as 10.0.0.0";
assert lib.mod (ipv4ToInt config.networkBase) networkSize == 0
  || throw "networkBase (${config.networkBase}) is not aligned to /${toString config.networkPrefixLength}";
assert config.masterDhcpIp == "MASTER_DHCP_IP"
  || ipv4ToInt config.masterDhcpIp < ipv4ToInt config.networkBase
  || ipv4ToInt config.masterDhcpIp >= ipv4ToInt config.networkBase + networkSize
  || throw "controller DHCP address must be outside the static laboratory network; choose a distinct laboratory subnet";
assert config.masterHostNumber < networkSize - 1
  || throw "masterHostNumber (${toString config.masterHostNumber}) does not fit in ${config.networkBase}/${toString config.networkPrefixLength}";
assert !checkTransition || isIpv4 transition.previousBase
  || throw "networkTransition.previousBase must be an IPv4 network address such as 10.0.0.0";
assert !checkTransition || lib.mod (ipv4ToInt transition.previousBase) previousNetworkSize == 0
  || throw "networkTransition.previousBase (${transition.previousBase}) is not aligned to /${toString transition.previousPrefixLength}";
assert !checkTransition
  || transition.previousBase != config.networkBase
  || transition.previousPrefixLength != config.networkPrefixLength
  || throw "networkTransition must name a previous network different from the current laboratory network";
assert !checkTransition || !transitionOverlaps
  || throw "networkTransition previous network ${transition.previousBase}/${toString transition.previousPrefixLength} must not overlap the laboratory network ${config.networkBase}/${toString config.networkPrefixLength}";
assert !checkTransition || config.masterHostNumber < previousNetworkSize - 1
  || throw "masterHostNumber (${toString config.masterHostNumber}) does not fit in the previous network ${transition.previousBase}/${toString transition.previousPrefixLength}";
assert !checkTransition
  || config.masterDhcpIp == "MASTER_DHCP_IP"
  || ipv4ToInt config.masterDhcpIp < ipv4ToInt transition.previousBase
  || ipv4ToInt config.masterDhcpIp >= ipv4ToInt transition.previousBase + previousNetworkSize
  || throw "controller DHCP address must be outside the previous laboratory network too";
assert !checkTransition || isRfc3339 transition.startedAt
  || throw "networkTransition.startedAt must be an RFC 3339 time such as 2026-10-15T08:00:00Z";
assert isIfaceName config.ifaceName
  || throw "ifaceName must be a valid Linux interface name of at most 15 characters";
assert config.controllerIfaceName == null || isIfaceName config.controllerIfaceName
  || throw "controllerIfaceName must be null or a valid Linux interface name of at most 15 characters";
assert config.clientIfaceName == null || isIfaceName config.clientIfaceName
  || throw "clientIfaceName must be null or a valid Linux interface name of at most 15 characters";
assert builtins.all isIfaceName (builtins.attrValues config.hostIfaceNames)
  || throw "hostIfaceNames values must be valid Linux interface names of at most 15 characters";
assert unknownInterfaceHosts == []
  || throw "hostIfaceNames contains unknown hosts: ${builtins.concatStringsSep ", " unknownInterfaceHosts}";
assert emptyRequiredFields == []
  || throw "settings must not be empty: ${builtins.concatStringsSep ", " emptyRequiredFields}";
assert isGitName config.studentGitName && isGitName config.adminGitName
  || throw "Git names must be at most 256 bytes without control characters";
assert isGitEmail config.studentGitEmail && isGitEmail config.adminGitEmail
  || throw "Git emails must be at most 256 bytes without control characters";
assert isKeyboardLayout config.keyboardLayout
  || throw "keyboardLayout must be an XKB layout name: letters, digits and _ - + , ( ) only, at most 128 bytes";
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
assert isAbsoluteHttpUrl config.homepageUrl
  || throw "homepageUrl must be an absolute http:// or https:// URL";
config
