{ ... }:

{
  # npm's default global prefix is inside the read-only Nix store. Keep global
  # packages in each user's home and expose their executables in every session.
  environment.extraInit = ''
    export NPM_CONFIG_PREFIX="$HOME/.local/npm"
    export PATH="$NPM_CONFIG_PREFIX/bin:$PATH"
  '';
}
