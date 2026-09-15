{ lib, pkgs }:
let
  definitions = [
    { id = "vlc"; label = "VLC"; summary = "Play video and audio files"; }
    { id = "gimp"; label = "GIMP"; summary = "Edit raster images"; }
    { id = "inkscape"; label = "Inkscape"; summary = "Create and edit vector graphics"; }
    { id = "krita"; label = "Krita"; summary = "Digital painting and illustration"; }
    { id = "blender"; label = "Blender"; summary = "3D modelling, animation and rendering"; }
    { id = "freecad"; label = "FreeCAD"; summary = "Parametric 3D CAD"; }
    { id = "audacity"; label = "Audacity"; summary = "Record and edit audio"; }
    { id = "obs-studio"; label = "OBS Studio"; summary = "Record and stream the desktop"; }
    { id = "filezilla"; label = "FileZilla"; summary = "Transfer files with FTP and SFTP"; }
    { id = "thunderbird"; label = "Thunderbird"; summary = "Email and calendar client"; }
    { id = "wireshark"; label = "Wireshark"; summary = "Inspect network traffic"; }
  ];
  available = definition:
    builtins.hasAttr definition.id pkgs
    && lib.isDerivation (builtins.getAttr definition.id pkgs)
    && lib.meta.availableOn pkgs.stdenv.hostPlatform (builtins.getAttr definition.id pkgs);
in
map (definition: definition // { availability = "available"; })
  (builtins.filter available definitions)
