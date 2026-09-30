{
  schemaVersion = 1;
  # No hidden application requirements. The inactive examples declare their
  # complete initial selection; omitted fields use system/application defaults.
  baseline = { schemaVersion = 1; };
  applications = [
    { id = "chromium-browser.desktop"; package = "chromium"; browser = true; }
    { id = "com.mitchellh.ghostty.desktop"; package = "ghostty"; }
    { id = "org.gnome.Nautilus.desktop"; package = "nautilus"; }
    { id = "org.gnome.TextEditor.desktop"; package = "gnome-text-editor"; }
    { id = "code.desktop"; package = "vscode"; }
    { id = "mysql-workbench.desktop"; package = "mysql-workbench"; }
  ];
  # Choices are not installations or loading/compatibility certificates.
  # requiredPackages are the language tools an extension expects on every
  # destination; requiredExtensions mirror its declared extension dependencies.
  # writable copies an extension that creates files in its own folder into
  # the home instead of linking it. Other extensions from the pinned package
  # set can be selected too; they are linked and need no entry here.
  extensions = [
    # Web
    { id = "ritwickdey.liveserver"; package = "vscode-extensions.ritwickdey.liveserver"; }
    { id = "ecmel.vscode-html-css"; package = "vscode-extensions.ecmel.vscode-html-css"; }
    { id = "esbenp.prettier-vscode"; package = "vscode-extensions.esbenp.prettier-vscode"; }
    # PHP
    { id = "bmewburn.vscode-intelephense-client"; package = "vscode-extensions.bmewburn.vscode-intelephense-client"; requiredPackages = [ "php" ]; }
    { id = "xdebug.php-debug"; package = "vscode-extensions.xdebug.php-debug"; requiredPackages = [ "php" ]; }
    # C and C++
    { id = "ms-vscode.cpptools"; package = "vscode-extensions.ms-vscode.cpptools"; requiredPackages = [ "gcc" "gdb" ]; }
    # Python
    { id = "ms-python.python"; package = "vscode-extensions.ms-python.python"; requiredPackages = [ "python3" ]; }
    { id = "ms-python.debugpy"; package = "vscode-extensions.ms-python.debugpy"; requiredExtensions = [ "ms-python.python" ]; writable = true; }
    { id = "ms-python.vscode-pylance"; package = "vscode-extensions.ms-python.vscode-pylance"; requiredExtensions = [ "ms-python.python" ]; }
    # Java
    { id = "redhat.java"; package = "vscode-extensions.redhat.java"; requiredPackages = [ "jdk21" ]; }
    { id = "vscjava.vscode-java-debug"; package = "vscode-extensions.vscjava.vscode-java-debug"; requiredExtensions = [ "redhat.java" ]; writable = true; }
    { id = "vscjava.vscode-java-test"; package = "vscode-extensions.vscjava.vscode-java-test"; requiredExtensions = [ "redhat.java" "vscjava.vscode-java-debug" ]; }
    { id = "vscjava.vscode-maven"; package = "vscode-extensions.vscjava.vscode-maven"; requiredPackages = [ "maven" ]; }
    { id = "vscjava.vscode-java-dependency"; package = "vscode-extensions.vscjava.vscode-java-dependency"; requiredExtensions = [ "redhat.java" ]; }
    { id = "vscjava.vscode-java-pack"; package = "vscode-extensions.vscjava.vscode-java-pack"; }
  ];
}
