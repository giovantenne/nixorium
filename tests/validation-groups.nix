# Keep related module graphs together, but release them between evaluators.
# ci-eval.nix checks that every declared Flake check occurs exactly once.
{
  checks-data = [
    "config-schema" "settings-schema" "software-schema" "software-preset-schema"
    "workspace-schema" "workspace-resolution" "desktop-profile"
    "workspace-seed" "workspace-seed-pinned"
  ];
  checks-lab = [ "mk-lab" ];
  checks-workspace-candidate = [ "workspace-candidate" ];
  checks-workspace-preparation = [ "workspace-preparation" ];
  checks-workspace-rejection = [ "workspace-rejection" ];
  checks-workspace-runtime = [ "workspace-runtime" ];
  checks-workspace-template = [ "workspace-template" ];
  checks-workspace-systems = [ "workspace-offline" "workspace-systems" ];
  checks-home-reset = [ "home-reset-filesystem-vm" "workspace-reset-service-vm" ];
  checks-editor = [ "workspace-editor-vm" "programming-profile-vm" ];
  checks-management = [ "management-vm" ];
  checks-session = [ "session-state-vm" ];
  checks-installer = [ "client-installer" "client-installer-vm" ];
  checks-remote-installer = [ "remote-client-installer-vm" ];
}
