// The classroom view shares the student's screen with the teacher. GNOME's
// sharing indicator stays visible as the notice, but it is no longer a stop
// button: a student cannot end the teacher's view from the top bar.
import * as Main from 'resource:///org/gnome/shell/ui/main.js';
import {Extension} from 'resource:///org/gnome/shell/extensions/extension.js';

export default class NixoriumClassroomExtension extends Extension {
    enable() {
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

    disable() {
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
