// Nixorium classroom extension, in every student session of a laboratory
// with the classroom view.
//
// The classroom view shares the student's screen with the teacher. GNOME's
// sharing indicator stays visible as the notice, but it is no longer a stop
// button: a student cannot end the teacher's view from the top bar.
//
// The teacher can also lock the computer: a full-screen cover takes the
// keyboard and the mouse until the teacher unlocks it. The classroom agent,
// in the same session, asks for it on the session bus. Logging out or
// restarting always ends the lock.
import Clutter from 'gi://Clutter';
import Gio from 'gi://Gio';
import GLib from 'gi://GLib';
import Shell from 'gi://Shell';
import St from 'gi://St';
import * as Main from 'resource:///org/gnome/shell/ui/main.js';
import {Extension} from 'resource:///org/gnome/shell/extensions/extension.js';

const BusName = 'org.nixorium.Classroom';
const ObjectPath = '/org/nixorium/Classroom';
const Interface = `<node>
  <interface name="org.nixorium.Classroom">
    <method name="SetLocked">
      <arg type="b" direction="in" name="locked"/>
    </method>
    <property name="Locked" type="b" access="read"/>
  </interface>
</node>`;

export default class NixoriumClassroomExtension extends Extension {
    enable() {
        this._quietIndicator();
        this._service = Gio.DBusExportedObject.wrapJSObject(Interface, this);
        this._service.export(Gio.DBus.session, ObjectPath);
        this._nameId = Gio.bus_own_name_on_connection(Gio.DBus.session, BusName,
            Gio.BusNameOwnerFlags.NONE, null, null);
    }

    disable() {
        this._setLocked(false);
        if (this._nameId) {
            Gio.bus_unown_name(this._nameId);
            this._nameId = 0;
        }
        this._service?.unexport();
        this._service = null;
        this._restoreIndicator();
    }

    // D-Bus: called by the classroom agent.
    SetLocked(locked) {
        this._setLocked(locked);
    }

    get Locked() {
        return Boolean(this._cover);
    }

    _setLocked(locked) {
        if (locked && !this._cover)
            this._lock();
        else if (!locked && this._cover)
            this._unlock();
        else
            return;
        this._service?.emit_property_changed('Locked', GLib.Variant.new_boolean(this.Locked));
    }

    _lock() {
        Main.overview.hide();
        Main.panel.menuManager.activeMenu?.close();
        const cover = new St.Widget({
            name: 'nixoriumClassroomLock',
            reactive: true,
            can_focus: true,
            style: 'background-color: #101317;',
            layout_manager: new Clutter.BinLayout(),
        });
        cover.add_constraint(new Clutter.BindConstraint({
            source: global.stage,
            coordinate: Clutter.BindCoordinate.ALL,
        }));
        cover.add_child(new St.Label({
            text: 'Eyes on the teacher',
            style: 'color: #ffffff; font-size: 48px; font-weight: bold;',
            x_align: Clutter.ActorAlign.CENTER,
            y_align: Clutter.ActorAlign.CENTER,
            x_expand: true,
            y_expand: true,
        }));
        Main.layoutManager.uiGroup.add_child(cover);
        Main.layoutManager.uiGroup.set_child_above_sibling(cover, null);
        // Keyboard and mouse go to the cover only; no shortcut works.
        this._grab = Main.pushModal(cover, {actionMode: Shell.ActionMode.NONE});
        cover.grab_key_focus();
        // Keep the cover above anything shown later, such as notifications.
        this._raiseId = Main.layoutManager.uiGroup.connect('child-added', () =>
            Main.layoutManager.uiGroup.set_child_above_sibling(cover, null));
        this._cover = cover;
    }

    _unlock() {
        Main.layoutManager.uiGroup.disconnect(this._raiseId);
        this._raiseId = 0;
        if (this._grab) {
            Main.popModal(this._grab);
            this._grab = null;
        }
        this._cover.destroy();
        this._cover = null;
    }

    _quietIndicator() {
        const indicator = Main.panel.statusArea.screenSharing;
        if (!indicator)
            return;
        this._indicator = indicator;
        this._reactive = indicator.reactive;
        this._canFocus = indicator.can_focus;
        indicator.reactive = false;
        indicator.can_focus = false;
        if (indicator._clickGesture)
            indicator._clickGesture.enabled = false;
        const stopIcon = indicator._box?.get_child_at_index(1) ?? null;
        if (stopIcon) {
            this._stopIcon = stopIcon;
            stopIcon.hide();
        }
    }

    _restoreIndicator() {
        const indicator = this._indicator;
        if (!indicator)
            return;
        indicator.reactive = this._reactive;
        indicator.can_focus = this._canFocus;
        if (indicator._clickGesture)
            indicator._clickGesture.enabled = true;
        this._stopIcon?.show();
        this._indicator = null;
        this._stopIcon = null;
    }
}
