{ lib, labSettings, hostSoftwarePackages, ... }:
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
    ++ lib.optional (has "starship") ''
      eval "$(starship init bash)"
    ''
    ++ lib.optional (has "zoxide") ''
      eval "$(zoxide init bash)"
    ''
  );
in
{
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
