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
//
// While the teacher shows their screen, a second cover above everything,
// the lock included, shows the latest picture the agent received, again
// with keyboard and mouse taken. When it ends, the lock (if any) remains.
//
// When an update from the controller replaces the running system, the system
// writes /run/nixorium/update-notice.json; a notification tells the person at
// the computer, with a Restart button when a restart finishes the update.
import Clutter from 'gi://Clutter';
import Cogl from 'gi://Cogl';
import GdkPixbuf from 'gi://GdkPixbuf';
import Gio from 'gi://Gio';
import GLib from 'gi://GLib';
import Shell from 'gi://Shell';
import St from 'gi://St';
import * as Main from 'resource:///org/gnome/shell/ui/main.js';
import * as MessageTray from 'resource:///org/gnome/shell/ui/messageTray.js';
import * as SystemActions from 'resource:///org/gnome/shell/misc/systemActions.js';
import {Extension} from 'resource:///org/gnome/shell/extensions/extension.js';

const NoticeDirectory = '/run/nixorium';
const NoticeFile = 'update-notice.json';
// An update without a needed restart is announced only while it is recent.
const RecentNoticeSeconds = 30 * 60;

const BusName = 'org.nixorium.Classroom';
const ObjectPath = '/org/nixorium/Classroom';
const Interface = `<node>
  <interface name="org.nixorium.Classroom">
    <method name="SetLocked">
      <arg type="b" direction="in" name="locked"/>
    </method>
    <property name="Locked" type="b" access="read"/>
    <method name="ShowFrame">
      <arg type="s" direction="in" name="path"/>
    </method>
    <method name="StopBroadcast"/>
    <property name="Broadcasting" type="b" access="read"/>
  </interface>
</node>`;

export default class NixoriumClassroomExtension extends Extension {
    enable() {
        this._quietIndicator();
        this._service = Gio.DBusExportedObject.wrapJSObject(Interface, this);
        this._service.export(Gio.DBus.session, ObjectPath);
        this._nameId = Gio.bus_own_name_on_connection(Gio.DBus.session, BusName,
            Gio.BusNameOwnerFlags.NONE, null, null);
        this._watchUpdates();
    }

    disable() {
        this._noticeMonitor?.cancel();
        this._noticeMonitor = null;
        this._notice?.destroy();
        this._notice = null;
        this.StopBroadcast();
        this._setLocked(false);
        if (this._nameId) {
            Gio.bus_unown_name(this._nameId);
            this._nameId = 0;
        }
        this._service?.unexport();
        this._service = null;
        this._restoreIndicator();
    }

    _watchUpdates() {
        const directory = Gio.File.new_for_path(NoticeDirectory);
        try {
            this._noticeMonitor = directory.monitor_directory(Gio.FileMonitorFlags.WATCH_MOVES, null);
            this._noticeMonitor.connect('changed', (_monitor, file, other) => {
                if (file.get_basename() === NoticeFile || other?.get_basename() === NoticeFile)
                    this._showUpdate(false);
            });
        } catch (error) {
            console.warn(`Nixorium: cannot watch ${NoticeDirectory}: ${error.message}`);
        }
        // An update made before this login (or while nobody was logged in).
        this._showUpdate(true);
    }

    _showUpdate(atLogin) {
        let notice;
        try {
            const [, contents] = Gio.File.new_for_path(`${NoticeDirectory}/${NoticeFile}`).load_contents(null);
            notice = JSON.parse(new TextDecoder().decode(contents));
        } catch {
            return;
        }
        if (typeof notice.updatedAt !== 'number' || notice.updatedAt === this._shownUpdate)
            return;
        const age = GLib.get_real_time() / 1e6 - notice.updatedAt;
        if (atLogin && !notice.restart && age > RecentNoticeSeconds)
            return;
        this._shownUpdate = notice.updatedAt;
        this._notice?.destroy();
        const body = notice.restart
            ? 'Restart it to finish the update. Save your work first.'
            : 'The changes are ready to use.';
        this._notice = new MessageTray.Notification({
            source: MessageTray.getSystemSource(),
            title: 'This computer was updated',
            body,
            isTransient: false,
            urgency: notice.restart ? MessageTray.Urgency.HIGH : MessageTray.Urgency.NORMAL,
        });
        if (notice.restart)
            this._notice.addAction('Restart', () => SystemActions.getDefault().activateRestart());
        this._notice.connect('destroy', () => {
            this._notice = null;
        });
        MessageTray.getSystemSource().addNotification(this._notice);
    }

    // D-Bus: called by the classroom agent.
    SetLocked(locked) {
        this._setLocked(locked);
    }

    get Locked() {
        return Boolean(this._cover);
    }

    get Broadcasting() {
        return Boolean(this._screen);
    }

    // D-Bus: shows a JPEG from the agent's private runtime folder.
    ShowFrame(path) {
        const runtime = GLib.get_user_runtime_dir();
        if (GLib.path_get_dirname(path) !== runtime || !GLib.path_get_basename(path).startsWith('nixorium-classroom-frame'))
            throw new Error('Unexpected picture');
        const pixbuf = GdkPixbuf.Pixbuf.new_from_file(path);
        if (!this._screen)
            this._showScreen();
        const content = St.ImageContent.new_with_preferred_size(pixbuf.width, pixbuf.height);
        content.set_bytes(
            global.stage.context.get_backend().get_cogl_context(),
            pixbuf.read_pixel_bytes(),
            pixbuf.get_has_alpha() ? Cogl.PixelFormat.RGBA_8888 : Cogl.PixelFormat.RGB_888,
            pixbuf.width,
            pixbuf.height,
            pixbuf.rowstride);
        this._picture.set_content(content);
    }

    StopBroadcast() {
        if (!this._screen)
            return;
        Main.layoutManager.uiGroup.disconnect(this._screenRaiseId);
        this._screenRaiseId = 0;
        if (this._screenGrab) {
            Main.popModal(this._screenGrab);
            this._screenGrab = null;
        }
        this._screen.destroy();
        this._screen = null;
        this._picture = null;
        this._service?.emit_property_changed('Broadcasting', GLib.Variant.new_boolean(false));
    }

    _showScreen() {
        Main.overview.hide();
        Main.panel.menuManager.activeMenu?.close();
        const screen = new St.Widget({
            name: 'nixoriumClassroomBroadcast',
            reactive: true,
            can_focus: true,
            style: 'background-color: #000000;',
            layout_manager: new Clutter.BinLayout(),
        });
        screen.add_constraint(new Clutter.BindConstraint({
            source: global.stage,
            coordinate: Clutter.BindCoordinate.ALL,
        }));
        const picture = new Clutter.Actor({
            content_gravity: Clutter.ContentGravity.RESIZE_ASPECT,
            x_expand: true,
            y_expand: true,
        });
        screen.add_child(picture);
        Main.layoutManager.uiGroup.add_child(screen);
        Main.layoutManager.uiGroup.set_child_above_sibling(screen, null);
        this._screenGrab = Main.pushModal(screen, {actionMode: Shell.ActionMode.NONE});
        screen.grab_key_focus();
        this._screenRaiseId = Main.layoutManager.uiGroup.connect('child-added', () =>
            Main.layoutManager.uiGroup.set_child_above_sibling(screen, null));
        this._screen = screen;
        this._picture = picture;
        this._service?.emit_property_changed('Broadcasting', GLib.Variant.new_boolean(true));
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
        this._raiseId = Main.layoutManager.uiGroup.connect('child-added', () => {
            Main.layoutManager.uiGroup.set_child_above_sibling(cover, null);
            if (this._screen)
                Main.layoutManager.uiGroup.set_child_above_sibling(this._screen, null);
        });
        if (this._screen)
            Main.layoutManager.uiGroup.set_child_above_sibling(this._screen, null);
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
