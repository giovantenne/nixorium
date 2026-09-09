{ pkgs, ... }:
{
  programs.neovim.enable = true;

  programs.git = {
    enable = true;
    config = {
      credential.helper = "";
      core.askPass = "";
    };
  };

  programs.starship = {
    enable = true;
    settings = {
      add_newline = true;
      command_timeout = 200;
      format = "$hostname$directory$git_branch$git_status$character";
      character = {
        error_symbol = "[✗](bold #fd6883)";
        success_symbol = "[❯](bold #f38d70)";
      };
      hostname = {
        ssh_only = false;
        format = "[$hostname](bold #f9cc6c):";
      };
      directory = {
        truncation_length = 2;
        truncation_symbol = "…/";
        style = "#e6d9db";
        repo_root_style = "bold #f38d70";
        repo_root_format = "[$repo_root]($repo_root_style)[$path]($style)[$read_only]($read_only_style) ";
      };
      git_branch = {
        format = "[$branch]($style) ";
        style = "italic #adda78";
      };
      git_status = {
        format = "[$all_status]($style)";
        style = "#85dacc";
        ahead = "⇡\${count} ";
        diverged = "⇕⇡\${ahead_count}⇣\${behind_count} ";
        behind = "⇣\${count} ";
        conflicted = " ";
        up_to_date = " ";
        untracked = "? ";
        modified = " ";
        stashed = "";
        staged = "";
        renamed = "";
        deleted = "";
      };
    };
  };

  programs.bash.completion.enable = true;
  programs.bash.shellAliases = {
    ls = "eza --icons --group-directories-first";
    lsa = "eza --icons -a --group-directories-first";
    lt = "eza --icons -T -L 2 --group-directories-first";
    lta = "eza --icons -a -T -L 2 --group-directories-first";
    ff = "fzf";
  };
  programs.bash.interactiveShellInit = ''
    source ${pkgs.fzf}/share/fzf/key-bindings.bash
    source ${pkgs.fzf}/share/fzf/completion.bash
    source ${pkgs.bash-completion}/share/bash-completion/bash_completion
    source ${pkgs.git}/share/bash-completion/completions/git
  '';

  programs.zoxide.enable = true;
  programs.zoxide.enableBashIntegration = true;
}
