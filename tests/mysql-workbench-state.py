# Test helper: reports what the student's desktop shows without reading
# pixels, so the test waits on the application's real state rather than on
# OCR. Exit status 0 means the condition holds now.
#   showing NAME   a MySQL Workbench widget whose name starts with NAME is
#                  on screen
#   gone NAME      Workbench runs and no such widget is on screen
#   desktop        the GNOME Activities overview is closed
import sys
import gi
gi.require_version("Atspi", "2.0")
from gi.repository import Atspi, Gio, GLib

def workbench():
    desktop = Atspi.get_desktop(0)
    for index in range(desktop.get_child_count()):
        app = desktop.get_child_at_index(index)
        if app is not None and app.get_name() == "mysql-workbench-bin":
            return app
    return None

def find(node, name, depth):
    if node is None or depth > 40:
        return False
    # The application node itself is never "showing"; hidden subtrees
    # (closed menus, other tabs, dismissed dialogs) are not on screen.
    if node.get_role() != Atspi.Role.APPLICATION:
        if not node.get_state_set().contains(Atspi.StateType.SHOWING):
            return False
        if (node.get_name() or "").startswith(name):
            return True
    return any(find(node.get_child_at_index(i), name, depth + 1) for i in range(node.get_child_count()))

def overview_active():
    bus = Gio.bus_get_sync(Gio.BusType.SESSION, None)
    reply = bus.call_sync("org.gnome.Shell", "/org/gnome/Shell", "org.freedesktop.DBus.Properties", "Get",
                          GLib.Variant("(ss)", ("org.gnome.Shell", "OverviewActive")), GLib.VariantType("(v)"),
                          Gio.DBusCallFlags.NONE, 10000, None)
    return reply.unpack()[0]

command = sys.argv[1]
if command in ("showing", "gone"):
    # A running Workbench is required either way: "gone" never holds merely
    # because the application is absent from the accessibility tree.
    app = workbench()
    ok = app is not None and find(app, sys.argv[2], 0) == (command == "showing")
elif command == "desktop":
    ok = not overview_active()
else:
    sys.exit(f"unknown condition {command}")
print(f"{command} {' '.join(sys.argv[2:])}: {'yes' if ok else 'no'}")
sys.exit(0 if ok else 1)
