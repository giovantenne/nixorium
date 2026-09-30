{ lib, pkgs, labSettings, hostSoftwarePackages, ... }:
let
  has = package: builtins.elem package hostSoftwarePackages;
  shellInit = builtins.concatStringsSep "\n" (
    lib.optional (has "fzf") ''
      source /run/current-system/sw/share/fzf/key-bindings.bash
      source /run/current-system/sw/share/fzf/completion.bash
    ''
    ++ lib.optional (has "bash-completion") ''
      source /run/current-system/sw/share/bash-completion/bash_completion
    ''
    ++ lib.optional (has "git") ''
      source /run/current-system/sw/share/bash-completion/completions/git
    ''
    ++ lib.optional (has "zoxide") ''
      eval "$(zoxide init bash)"
    ''
    ++ [ ''
      PS1='\w \$ '
    '' ]
  );
  webRoot = "/home/${labSettings.studentUser}/public_html";
  # Step debugging stays idle until the editor asks for it.
  phpWithDebugger = pkgs.php.buildEnv {
    extensions = { enabled, all }: enabled ++ [ all.xdebug ];
    extraConfig = "xdebug.mode = debug";
  };
in
{
  # A classroom teaching database, not a shared or persistent service: it
  # listens on this computer only, the administrative account has no password
  # (the convention used by most course material), and all databases are
  # discarded at every boot like the student home.
  services.mysql = lib.mkIf (has "mysql84") {
    enable = true;
    package = pkgs.mysql84;
    secureSuperUserByDefault = false;
    settings.mysqld = {
      bind-address = "127.0.0.1";
      mysqlx-bind-address = "127.0.0.1";
    };
  };
  # The "!" limits removal to boot: a rebuild never empties a running server.
  systemd.tmpfiles.rules = lib.optional (has "mysql84") "R! /var/lib/mysql - - - - -";

  # A XAMPP-style local web server for exercises: http://localhost/ serves
  # the student's ~/public_html with PHP, .htaccess and folder listings. It
  # runs as the student account, so pages reach only that student's files; it
  # listens on this computer only, and the folder is reset with the home. On
  # the controller it also serves the student account's folder.
  services.httpd = lib.mkIf (has "apacheHttpd") {
    enable = true;
    user = labSettings.studentUser;
    group = "users";
    adminAddr = "webmaster@localhost";
    enablePHP = has "php";
    phpPackage = phpWithDebugger;
    virtualHosts.localhost = {
      listen = [
        { ip = "127.0.0.1"; port = 80; }
        { ip = "[::1]"; port = 80; }
      ];
      documentRoot = webRoot;
      extraConfig = ''
        <Directory "${webRoot}">
          Options Indexes FollowSymLinks
          AllowOverride All
          Require all granted
        </Directory>
        DirectoryIndex index.php index.html
      '';
    };
  };
  # The boot reset recreates the home first; then the empty folder is added.
  systemd.services.httpd = lib.mkIf (has "apacheHttpd") {
    after = [ "home-reset.service" ];
    serviceConfig.ExecStartPre = [
      "+${pkgs.coreutils}/bin/install -d -o ${labSettings.studentUser} -g users -m 0755 ${webRoot}"
    ];
  };

  # The selected interpreter stays declared; this variant wins on PATH. Set
  # the priority directly: lib.hiPrio rebuilds the environment through
  # overrideAttrs, which drops every enabled PHP extension.
  environment.systemPackages = lib.optional (has "php")
    (phpWithDebugger // { meta = phpWithDebugger.meta // { priority = 4; }; });

  virtualisation.docker.rootless = lib.mkIf (has "docker") {
    enable = true;
    setSocketVariable = true;
  };
  users.users = lib.mkIf (has "docker") {
    admin.autoSubUidGidRange = true;
    ${labSettings.teacherUser}.autoSubUidGidRange = true;
    ${labSettings.studentUser}.autoSubUidGidRange = true;
  };

  environment.extraInit = lib.mkIf (has "nodejs") ''
    export NPM_CONFIG_PREFIX="$HOME/.local/npm"
    export PATH="$NPM_CONFIG_PREFIX/bin:$PATH"
  '';

  environment.etc."gitconfig" = lib.mkIf (has "git") {
    text = ''
      [credential]
        helper =
      [core]
        askPass =
    '';
  };

  programs.bash.completion.enable = has "bash-completion";
  programs.bash.shellAliases = lib.optionalAttrs (has "eza") {
    ls = "eza --icons --group-directories-first";
    lsa = "eza --icons -a --group-directories-first";
    lt = "eza --icons -T -L 2 --group-directories-first";
    lta = "eza --icons -a -T -L 2 --group-directories-first";
  } // lib.optionalAttrs (has "fzf") {
    ff = "fzf";
  };
  programs.bash.interactiveShellInit = shellInit;
}
