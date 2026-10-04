# Test helper: presses Super with Mutter's remote desktop interface, as a
# student's keyboard would, and succeeds only if GNOME's overview opened
# (it is closed again afterwards).
import sys, time
from gi.repository import Gio, GLib

RD = "org.gnome.Mutter.RemoteDesktop"
bus = Gio.bus_get_sync(Gio.BusType.SESSION, None)
def call(name, path, iface, method, params, reply):
    return bus.call_sync(name, path, iface, method, params, GLib.VariantType(reply) if reply else None, Gio.DBusCallFlags.NONE, 10000, None)
def overview():
    return call("org.gnome.Shell", "/org/gnome/Shell", "org.freedesktop.DBus.Properties", "Get", GLib.Variant("(ss)", ("org.gnome.Shell", "OverviewActive")), "(v)").unpack()[0]
def key(keysym):
    for pressed in (True, False):
        call(RD, path, RD + ".Session", "NotifyKeyboardKeysym", GLib.Variant("(ub)", (keysym, pressed)), None)
path = call(RD, "/org/gnome/Mutter/RemoteDesktop", RD, "CreateSession", None, "(o)").unpack()[0]
call(RD, path, RD + ".Session", "Start", None, None)
time.sleep(1)
# The virtual keyboard appears with the first key: start with a neutral Shift.
key(0xffe1)
time.sleep(0.5)
key(0xffeb)
time.sleep(1.5)
opened = overview()
if opened:
    key(0xff1b)
    time.sleep(1)
call(RD, path, RD + ".Session", "Stop", None, None)
print("overview", opened)
sys.exit(0 if opened else 1)
