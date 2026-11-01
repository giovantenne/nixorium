# Test helper: presses F11 with Mutter's remote desktop interface, as a
# student's keyboard would, to switch the focused window to or from full
# screen.
import time
from gi.repository import Gio, GLib

RD = "org.gnome.Mutter.RemoteDesktop"
bus = Gio.bus_get_sync(Gio.BusType.SESSION, None)
def call(name, path, iface, method, params, reply):
    return bus.call_sync(name, path, iface, method, params, GLib.VariantType(reply) if reply else None, Gio.DBusCallFlags.NONE, 10000, None)
def key(keysym):
    for pressed in (True, False):
        call(RD, path, RD + ".Session", "NotifyKeyboardKeysym", GLib.Variant("(ub)", (keysym, pressed)), None)
path = call(RD, "/org/gnome/Mutter/RemoteDesktop", RD, "CreateSession", None, "(o)").unpack()[0]
call(RD, path, RD + ".Session", "Start", None, None)
time.sleep(1)
# The virtual keyboard appears with the first key: start with a neutral Shift.
key(0xffe1)
time.sleep(0.5)
key(0xffc8)
time.sleep(1)
call(RD, path, RD + ".Session", "Stop", None, None)
