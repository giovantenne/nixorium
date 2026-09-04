# Overlay for gnome-remote-desktop: enable VNC backend and allow Veyon
# reconnections by tearing down the previous session cleanly.
#
# Veyon's "External VNC Server" plugin connects to grd's VNC port (5900)
# each time the master requests a screen view.  The upstream code in
# grd-vnc-server.c refuses a second connection while one already exists.
#
# Simply allowing the new connection through (without stopping the old
# session) causes framebuffer corruption: two rfbScreen instances and two
# PipeWire capture streams run simultaneously, resulting in overlapping
# frames and rendering artifacts.
#
# The correct fix: when a new VNC client connects and there is already an
# active session, stop the existing session first (destroying its
# rfbScreen, framebuffer, and PipeWire stream), then accept the new one.
# This mirrors the pattern used in grd_vnc_server_stop().
{ prev }:

prev.gnome-remote-desktop.overrideAttrs (oldAttrs: {
  buildInputs = (oldAttrs.buildInputs or []) ++ [
    prev.libvncserver
  ];

  mesonFlags = (oldAttrs.mesonFlags or []) ++ [
    "-Dvnc=true"
  ];

  postPatch = (oldAttrs.postPatch or "") + ''
    # GNOME 50 moved its single-client policy into GrdThrottler. Let a second
    # connection reach the callback, then replace the existing VNC session.
    substituteInPlace src/grd-vnc-server.c \
      --replace-fail \
        '  g_debug ("Creating new VNC session");' \
        '  while (vnc_server->sessions)
    {
      GrdSession *existing = vnc_server->sessions->data;

      g_debug ("Stopping existing VNC session for reconnect");
      grd_session_stop (existing);
    }

  grd_vnc_server_cleanup_stopped_sessions (vnc_server);
  g_debug ("Creating new VNC session");' \
      --replace-fail \
        'grd_throttler_limits_set_max_global_connections (limits, 1);' \
        'grd_throttler_limits_set_max_global_connections (limits, 2);'
  '';
})
