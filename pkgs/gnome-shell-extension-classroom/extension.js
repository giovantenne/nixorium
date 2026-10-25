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
// The teacher can also show a message: a centered panel above everything,
// the lock and the teacher's screen included, with plain text only. It does
// not take the keyboard or the mouse; the student keeps them unless the
// computer is locked or shows the teacher's screen. A message stays until the
// student closes it (the Close button appears after a few seconds), for a
// number of seconds, or until the computer is unlocked. A new message
// replaces the previous one, and logging out removes it.
//
// While the teacher controls the computer with their keyboard and mouse, a
// small label under the top bar says so. It is plain text above everything,
// does not take clicks or keys, and goes when the control ends.
//
// While the screen is being shared and a full-screen window hides GNOME's top
// bar (a video, a game), a small "Classroom view open" label in a corner of
// the screen takes the place of the sharing icon. It takes no clicks or keys.
//
// When the teacher saves a picture of the screen or collects the files on
// the desktop, a small notice in the bottom corner says so for a few
// seconds and fades. The agent sends only the kind; the words are here. The notice
// takes no clicks or keys.
//
// When an update from the controller replaces the running system, the system
// writes /run/nixorium/update-notice.json; a notification tells the person at
// the computer, with a Restart button when a restart finishes the update.
import Clutter from 'gi://Clutter';
import Cogl from 'gi://Cogl';
import GdkPixbuf from 'gi://GdkPixbuf';
import Gio from 'gi://Gio';
import GLib from 'gi://GLib';
import Pango from 'gi://Pango';
import Shell from 'gi://Shell';
import St from 'gi://St';
import System from 'system';
import * as Main from 'resource:///org/gnome/shell/ui/main.js';
import * as MessageTray from 'resource:///org/gnome/shell/ui/messageTray.js';
import * as SystemActions from 'resource:///org/gnome/shell/misc/systemActions.js';
import {Extension} from 'resource:///org/gnome/shell/extensions/extension.js';

const NoticeDirectory = '/run/nixorium';
const NoticeFile = 'update-notice.json';
// An update without a needed restart is announced only while it is recent.
const RecentNoticeSeconds = 30 * 60;
// A message that stays until the student closes it offers Close after this.
const MessageCloseSeconds = 10;
// The notice of a screenshot or a collection stays this long.
const NoticeSeconds = 6;
const NoticeTexts = {
    screenshot: 'Your teacher saved a picture of this screen',
    collect: 'Your teacher collected the files on your desktop',
};

// Decoded pictures of the teacher's screen live in native memory that the
// JavaScript collector does not count: without a collection every few
// pictures, a few seconds at fifteen a second fill the computer's memory.
const CollectAfterBytes = 64 * 1024 * 1024;

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
    <method name="ShowMessage">
      <arg type="s" direction="in" name="text"/>
      <arg type="i" direction="in" name="seconds"/>
      <arg type="b" direction="in" name="untilUnlock"/>
    </method>
    <method name="HideMessage"/>
    <property name="MessageShown" type="b" access="read"/>
    <method name="SetControlled">
      <arg type="b" direction="in" name="controlled"/>
    </method>
    <property name="Controlled" type="b" access="read"/>
    <method name="SetSharing">
      <arg type="b" direction="in" name="sharing"/>
    </method>
    <property name="IndicatorShown" type="b" access="read"/>
    <method name="ShowNotice">
      <arg type="s" direction="in" name="kind"/>
    </method>
    <property name="NoticeKind" type="s" access="read"/>
  </interface>
</node>`;

export default class NixoriumClassroomExtension extends Extension {
    enable() {
        this._quietIndicator();
        // GNOME hides the top bar, and with it the sharing icon, while a
        // window is full screen: the small indicator takes over then.
        Main.layoutManager.panelBox.connectObject('notify::visible', () => this._updateChip(), this);
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
        this.HideMessage();
        this.SetControlled(false);
        this._hideNotice();
        this._sharing = false;
        this._updateChip();
        Main.layoutManager.panelBox.disconnectObject(this);
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
            : 'The update is in place; programs you open from now on use it. If something does not work as expected, restart the computer.';
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
        // One content for the whole showing; a new size gets a new one.
        const content = this._picture.get_content();
        if (content?.get_preferred_size()[1] !== pixbuf.width || content?.get_preferred_size()[2] !== pixbuf.height)
            this._picture.set_content(St.ImageContent.new_with_preferred_size(pixbuf.width, pixbuf.height));
        this._picture.get_content().set_bytes(
            global.stage.context.get_backend().get_cogl_context(),
            pixbuf.read_pixel_bytes(),
            pixbuf.get_has_alpha() ? Cogl.PixelFormat.RGBA_8888 : Cogl.PixelFormat.RGB_888,
            pixbuf.width,
            pixbuf.height,
            pixbuf.rowstride);
        // The pixels and their copy for set_bytes.
        this._decodedBytes = (this._decodedBytes ?? 0) + 2 * pixbuf.rowstride * pixbuf.height;
        if (this._decodedBytes >= CollectAfterBytes) {
            this._decodedBytes = 0;
            System.gc();
        }
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
        this._decodedBytes = 0;
        System.gc();
        this._service?.emit_property_changed('Broadcasting', GLib.Variant.new_boolean(false));
        this._updateMessage();
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
        this._screenRaiseId = Main.layoutManager.uiGroup.connect('child-added', () => {
            Main.layoutManager.uiGroup.set_child_above_sibling(screen, null);
            this._updateMessage();
        });
        this._screen = screen;
        this._picture = picture;
        this._service?.emit_property_changed('Broadcasting', GLib.Variant.new_boolean(true));
        this._updateMessage();
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
            this._updateMessage();
        });
        if (this._screen)
            Main.layoutManager.uiGroup.set_child_above_sibling(this._screen, null);
        this._cover = cover;
        this._updateMessage();
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
        // A message that was to stay until the unlock ends here.
        if (this._message?.untilUnlock)
            this.HideMessage();
        else
            this._updateMessage();
    }

    // D-Bus: called by the classroom agent, which has checked the text and
    // the duration: seconds is 0 or a number of seconds, and untilUnlock
    // excludes seconds.
    ShowMessage(text, seconds, untilUnlock) {
        this.HideMessage();
        // The panel sits in a transparent layer that the pointer passes
        // through; only the panel itself takes clicks.
        const layer = new St.Widget({
            name: 'nixoriumClassroomMessage',
            layout_manager: new Clutter.BinLayout(),
        });
        layer.add_constraint(new Clutter.BindConstraint({
            source: global.stage,
            coordinate: Clutter.BindCoordinate.ALL,
        }));
        const panel = new St.BoxLayout({
            orientation: Clutter.Orientation.VERTICAL,
            reactive: true,
            x_align: Clutter.ActorAlign.CENTER,
            y_align: Clutter.ActorAlign.CENTER,
            x_expand: true,
            y_expand: true,
            style: 'background-color: #20242b; border: 2px solid #78aef0; border-radius: 16px; padding: 32px 40px; spacing: 20px; max-width: 760px;',
        });
        panel.add_child(new St.Label({
            text: 'Message from your teacher',
            style: 'color: #a2aab6; font-size: 16px;',
            x_align: Clutter.ActorAlign.CENTER,
        }));
        // St.Label shows plain text, never markup.
        const label = new St.Label({
            text,
            style: 'color: #ffffff; font-size: 28px; text-align: center;',
            x_align: Clutter.ActorAlign.CENTER,
        });
        label.clutter_text.set({
            line_wrap: true,
            line_wrap_mode: Pango.WrapMode.WORD_CHAR,
            ellipsize: Pango.EllipsizeMode.NONE,
        });
        panel.add_child(label);
        // Without a lock to end it, "until unlocked" lets the student close
        // the message instead.
        const untilLock = untilUnlock && Boolean(this._cover);
        const closable = seconds === 0 && !untilLock;
        let close = null;
        if (closable) {
            close = new St.Button({
                label: 'Close',
                // Its place is kept from the start, so the panel does not
                // change size when Close appears (see _updateMessage).
                opacity: 0,
                reactive: false,
                can_focus: true,
                x_align: Clutter.ActorAlign.CENTER,
                style: 'background-color: #78aef0; color: #101317; font-size: 18px; font-weight: bold; padding: 8px 32px; border-radius: 8px;',
            });
            close.connect('clicked', () => this.HideMessage());
            panel.add_child(close);
        }
        layer.add_child(panel);
        Main.layoutManager.uiGroup.add_child(layer);
        const message = {
            layer,
            close,
            // Close is offered once this delay has passed.
            ready: false,
            untilUnlock: untilLock,
            timer: 0,
            raiseId: Main.layoutManager.uiGroup.connect('child-added', () => this._updateMessage()),
        };
        if (seconds > 0) {
            message.timer = GLib.timeout_add_seconds(GLib.PRIORITY_DEFAULT, Math.min(seconds, 600), () => {
                message.timer = 0;
                this.HideMessage();
                return GLib.SOURCE_REMOVE;
            });
        } else if (closable) {
            message.timer = GLib.timeout_add_seconds(GLib.PRIORITY_DEFAULT, MessageCloseSeconds, () => {
                message.timer = 0;
                message.ready = true;
                this._updateMessage();
                return GLib.SOURCE_REMOVE;
            });
        }
        this._message = message;
        this._updateMessage();
        this._service?.emit_property_changed('MessageShown', GLib.Variant.new_boolean(true));
    }

    HideMessage() {
        const message = this._message;
        if (!message)
            return;
        this._message = null;
        if (message.timer)
            GLib.source_remove(message.timer);
        Main.layoutManager.uiGroup.disconnect(message.raiseId);
        message.layer.destroy();
        this._service?.emit_property_changed('MessageShown', GLib.Variant.new_boolean(false));
    }

    get MessageShown() {
        return Boolean(this._message);
    }

    // Keeps the message above everything else, and offers Close only while
    // the student can click: a lock or the teacher's screen takes the mouse.
    _updateMessage() {
        const message = this._message;
        if (!message)
            return;
        Main.layoutManager.uiGroup.set_child_above_sibling(message.layer, null);
        if (message.close) {
            const offered = message.ready && !this._cover && !this._screen;
            message.close.opacity = offered ? 255 : 0;
            message.close.reactive = offered;
        }
    }

    // D-Bus: called by the classroom agent while the teacher's keyboard and
    // mouse control this computer.
    SetControlled(controlled) {
        if (controlled === Boolean(this._control))
            return;
        if (!controlled) {
            Main.layoutManager.uiGroup.disconnect(this._control.raiseId);
            this._control.layer.destroy();
            this._control = null;
        } else {
            const layer = new St.Widget({
                name: 'nixoriumClassroomControl',
                layout_manager: new Clutter.BinLayout(),
            });
            layer.add_constraint(new Clutter.BindConstraint({
                source: global.stage,
                coordinate: Clutter.BindCoordinate.ALL,
            }));
            layer.add_child(new St.Label({
                text: 'Teacher in control',
                style: 'background-color: rgba(180, 83, 9, 0.92); color: #ffffff; font-size: 13px; font-weight: bold; padding: 3px 12px; border-radius: 10px; margin-top: 38px;',
                x_align: Clutter.ActorAlign.CENTER,
                y_align: Clutter.ActorAlign.START,
                x_expand: true,
                y_expand: true,
            }));
            Main.layoutManager.uiGroup.add_child(layer);
            Main.layoutManager.uiGroup.set_child_above_sibling(layer, null);
            const raiseId = Main.layoutManager.uiGroup.connect('child-added', () => {
                Main.layoutManager.uiGroup.set_child_above_sibling(layer, null);
                this._updateMessage();
            });
            this._control = {layer, raiseId};
        }
        this._service?.emit_property_changed('Controlled', GLib.Variant.new_boolean(Boolean(this._control)));
    }

    get Controlled() {
        return Boolean(this._control);
    }

    // D-Bus: called by the classroom agent, which has checked the kind.
    ShowNotice(kind) {
        const text = NoticeTexts[kind];
        if (!text)
            throw new Error('Unexpected notice');
        this._hideNotice();
        const layer = new St.Widget({
            name: 'nixoriumClassroomNotice',
            layout_manager: new Clutter.BinLayout(),
        });
        layer.add_constraint(new Clutter.BindConstraint({
            source: global.stage,
            coordinate: Clutter.BindCoordinate.ALL,
        }));
        layer.add_child(new St.Label({
            text,
            style: 'background-color: rgba(32, 36, 43, 0.92); color: #ffffff; font-size: 14px; padding: 6px 14px; border-radius: 10px; margin: 0 24px 24px 0;',
            x_align: Clutter.ActorAlign.END,
            y_align: Clutter.ActorAlign.END,
            x_expand: true,
            y_expand: true,
        }));
        Main.layoutManager.uiGroup.add_child(layer);
        Main.layoutManager.uiGroup.set_child_above_sibling(layer, null);
        const raiseId = Main.layoutManager.uiGroup.connect('child-added', () => {
            Main.layoutManager.uiGroup.set_child_above_sibling(layer, null);
            this._updateMessage();
        });
        const notice = {kind, layer, raiseId, timer: 0};
        // It fades in and out; nothing else moves.
        layer.opacity = 0;
        layer.ease({opacity: 255, duration: 300, mode: Clutter.AnimationMode.EASE_OUT_QUAD});
        notice.timer = GLib.timeout_add_seconds(GLib.PRIORITY_DEFAULT, NoticeSeconds, () => {
            notice.timer = 0;
            layer.ease({
                opacity: 0,
                duration: 600,
                mode: Clutter.AnimationMode.EASE_OUT_QUAD,
                onComplete: () => {
                    if (this._noticeShown === notice)
                        this._hideNotice();
                },
            });
            return GLib.SOURCE_REMOVE;
        });
        this._noticeShown = notice;
        this._service?.emit_property_changed('NoticeKind', GLib.Variant.new_string(kind));
    }

    get NoticeKind() {
        return this._noticeShown?.kind ?? '';
    }

    _hideNotice() {
        const notice = this._noticeShown;
        if (!notice)
            return;
        this._noticeShown = null;
        if (notice.timer)
            GLib.source_remove(notice.timer);
        Main.layoutManager.uiGroup.disconnect(notice.raiseId);
        notice.layer.destroy();
        this._service?.emit_property_changed('NoticeKind', GLib.Variant.new_string(''));
    }

    // D-Bus: called by the classroom agent while a capture session exists.
    SetSharing(sharing) {
        this._sharing = Boolean(sharing);
        this._updateChip();
    }

    get IndicatorShown() {
        return Boolean(this._chip);
    }

    // The top bar is hidden exactly when a window is full screen on the
    // primary monitor (layout.js hides the panel box then).
    _updateChip() {
        const wanted = Boolean(this._sharing) && !Main.layoutManager.panelBox.visible;
        if (wanted === Boolean(this._chip))
            return;
        if (!wanted) {
            Main.layoutManager.uiGroup.disconnect(this._chip.raiseId);
            this._chip.layer.destroy();
            this._chip = null;
        } else {
            const layer = new St.Widget({
                name: 'nixoriumClassroomIndicator',
                layout_manager: new Clutter.BinLayout(),
            });
            layer.add_constraint(new Clutter.BindConstraint({
                source: global.stage,
                coordinate: Clutter.BindCoordinate.ALL,
            }));
            layer.add_child(new St.Label({
                text: 'Classroom view open',
                style: 'background-color: rgba(16, 19, 23, 0.7); color: #ffffff; font-size: 13px; padding: 4px 12px; border-radius: 0 0 0 8px;',
                x_align: Clutter.ActorAlign.END,
                y_align: Clutter.ActorAlign.START,
                x_expand: true,
                y_expand: true,
            }));
            Main.layoutManager.uiGroup.add_child(layer);
            Main.layoutManager.uiGroup.set_child_above_sibling(layer, null);
            // Kept above anything shown later, the way the lock cover is.
            const raiseId = Main.layoutManager.uiGroup.connect('child-added', () => {
                Main.layoutManager.uiGroup.set_child_above_sibling(layer, null);
                this._updateMessage();
            });
            this._chip = {layer, raiseId};
        }
        this._service?.emit_property_changed('IndicatorShown', GLib.Variant.new_boolean(Boolean(this._chip)));
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
