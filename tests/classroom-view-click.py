# Test helper: clicks the top bar sharing indicator with Mutter's remote
# desktop interface, as a student's mouse would, and reports whether every
# remote desktop session (including the agent's) is still alive afterwards.
import sys, time
from gi.repository import Gio, GLib

RD = "org.gnome.Mutter.RemoteDesktop"
SC = "org.gnome.Mutter.ScreenCast"
x, y = float(sys.argv[1]), float(sys.argv[2])
bus = Gio.bus_get_sync(Gio.BusType.SESSION, None)
def call(name, path, iface, method, params, reply):
    return bus.call_sync(name, path, iface, method, params, GLib.VariantType(reply) if reply else None, Gio.DBusCallFlags.NONE, 10000, None)
def sessions():
    xml = call(RD, "/org/gnome/Mutter/RemoteDesktop/Session", "org.freedesktop.DBus.Introspectable", "Introspect", None, "(s)").unpack()[0]
    return sorted(node.split('"')[1] for node in xml.split("<node name=")[1:])
before = sessions()
path = call(RD, "/org/gnome/Mutter/RemoteDesktop", RD, "CreateSession", None, "(o)").unpack()[0]
sid = call(RD, path, "org.freedesktop.DBus.Properties", "Get", GLib.Variant("(ss)", (RD + ".Session", "SessionId")), "(v)").unpack()[0]
cast = call(SC, "/org/gnome/Mutter/ScreenCast", SC, "CreateSession", GLib.Variant("(a{sv})", ({"remote-desktop-session-id": GLib.Variant("s", sid)},)), "(o)").unpack()[0]
stream = call(SC, cast, SC + ".Session", "RecordMonitor", GLib.Variant("(sa{sv})", ("", {})), "(o)").unpack()[0]
call(RD, path, RD + ".Session", "Start", None, None)
time.sleep(2)
call(RD, path, RD + ".Session", "NotifyPointerMotionAbsolute", GLib.Variant("(sdd)", (stream, x, y)), None)
for pressed in (True, False):
    call(RD, path, RD + ".Session", "NotifyPointerButton", GLib.Variant("(ib)", (0x110, pressed)), None)
time.sleep(2)
after = sessions()
print("before", before, "after", after)
sys.exit(0 if set(before) <= set(after) else 1)
