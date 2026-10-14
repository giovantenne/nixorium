# Test helper: moves Mutter's remote desktop pointer and clicks, as a
# teacher's mouse would. Arguments: X Y [CLICKS], one pointer action per call.
import sys, time
from gi.repository import Gio, GLib

RD = "org.gnome.Mutter.RemoteDesktop"
SC = "org.gnome.Mutter.ScreenCast"
x, y = float(sys.argv[1]), float(sys.argv[2])
clicks = int(sys.argv[3]) if len(sys.argv) > 3 else 1
bus = Gio.bus_get_sync(Gio.BusType.SESSION, None)
def call(name, path, iface, method, params, reply):
    return bus.call_sync(name, path, iface, method, params, GLib.VariantType(reply) if reply else None, Gio.DBusCallFlags.NONE, 10000, None)
path = call(RD, "/org/gnome/Mutter/RemoteDesktop", RD, "CreateSession", None, "(o)").unpack()[0]
sid = call(RD, path, "org.freedesktop.DBus.Properties", "Get", GLib.Variant("(ss)", (RD + ".Session", "SessionId")), "(v)").unpack()[0]
cast = call(SC, "/org/gnome/Mutter/ScreenCast", SC, "CreateSession", GLib.Variant("(a{sv})", ({"remote-desktop-session-id": GLib.Variant("s", sid)},)), "(o)").unpack()[0]
stream = call(SC, cast, SC + ".Session", "RecordMonitor", GLib.Variant("(sa{sv})", ("", {})), "(o)").unpack()[0]
call(RD, path, RD + ".Session", "Start", None, None)
time.sleep(2)
call(RD, path, RD + ".Session", "NotifyPointerMotionAbsolute", GLib.Variant("(sdd)", (stream, x, y)), None)
time.sleep(0.5)
for _ in range(clicks):
    for pressed in (True, False):
        call(RD, path, RD + ".Session", "NotifyPointerButton", GLib.Variant("(ib)", (0x110, pressed)), None)
    time.sleep(0.1)
time.sleep(1)
call(RD, path, RD + ".Session", "Stop", None, None)
