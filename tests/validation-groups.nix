# Keep related module graphs together, but release them between evaluators.
# ci-eval.nix checks that every declared Flake check occurs exactly once.
{
  checks-data = [
    "config-schema" "settings-schema" "software-schema" "software-preset-schema"
    "workspace-schema" "home-files-schema" "home-files-seed" "workspace-resolution" "desktop-profile"
    "workspace-seed" "workspace-seed-pinned"
  ];
  checks-lab = [ "mk-lab" ];
  checks-workspace-candidate = [ "workspace-candidate" ];
  checks-workspace-preparation = [ "workspace-preparation" ];
  checks-workspace-rejection = [ "workspace-rejection" ];
  checks-workspace-runtime = [ "workspace-runtime" ];
  checks-workspace-template = [ "workspace-template" ];
  checks-workspace-systems = [ "workspace-offline" "workspace-systems" "home-files-adoption" ];
  checks-home-reset = [ "home-reset-filesystem-vm" "workspace-reset-service-vm" ];
  checks-editor = [ "workspace-editor-vm" "programming-profile-vm" "mysql-workbench-vm" ];
  checks-management = [ "management-vm" "network-migration-vm" "assistant-vm" "host-diagnostics-vm" "telemetry-vm" "onboarding-vm" ];
  checks-session = [ "console-keyboard-vm" "session-state-vm" "clean-generations-vm" "classroom-view-vm" ];
  checks-installer = [ "client-installer" "client-installer-vm" "client-network-upgrade-vm" "class-network-vm" "network-transition-vm" ];
  checks-remote-installer = [ "remote-client-installer-vm" ];
}
