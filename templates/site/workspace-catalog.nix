{
  schemaVersion = 1;
  # No hidden application requirements. The inactive example declares its
  # complete initial selection; omitted fields use system/application defaults.
  baseline = { schemaVersion = 1; };
  applications = [
    { id = "chromium-browser.desktop"; package = "chromium"; browser = true; }
    { id = "com.mitchellh.ghostty.desktop"; package = "ghostty"; }
    { id = "org.gnome.Nautilus.desktop"; package = "nautilus"; }
    { id = "org.gnome.TextEditor.desktop"; package = "gnome-text-editor"; }
    { id = "code.desktop"; package = "vscode"; }
  ];
  # Choices are not installations or loading/compatibility certificates.
  # Review runtime and extension dependencies before expanding this catalog.
  extensions = [
    { id = "ritwickdey.liveserver"; package = "vscode-extensions.ritwickdey.liveserver"; }
  ];
}
