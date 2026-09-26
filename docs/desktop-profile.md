# Minimal GNOME desktop profile

The deployment-owned workstation module supplies a restrained GNOME 50 profile:

- Native dark Adwaita decorations with blue accents, plus MoreWaita application
  and MIME icons. GTK 4 applications retain their supported native styling.
- A static vector background without animation or blur.
- A compact, visible bottom Dash to Dock with 40 px icons, no full-width panel,
  and no duplicate trash or removable-drive entries.
- Desktop Icons NG for files saved to the Desktop directory.
- Tiling Assistant for snap assist and 8 px window gaps, with its panel indicator
  disabled. Existing Super+arrow window controls remain available.

There are three managed Shell extensions in total. Packages, extension metadata
and schemas come from the deployment's locked Nixpkgs; nothing is fetched at
login. Appearance remains site policy, not an upstream GNOME mechanism.

## Research and tradeoffs

Reviewed on 2026-09-26. User feedback favors coherent icon coverage, while
reports about extension interactions favor keeping the stack small. For
example, the [MoreWaita release discussion](https://www.reddit.com/r/gnome/comments/1g6cdn1/)
praises desktop consistency; that is subjective feedback, not a benchmark.
The [maintainer's description](https://github.com/somepaulo/MoreWaita) explains
that MoreWaita complements Adwaita and preserves native GNOME/Circle icons.

[Tiling Assistant](https://github.com/ubuntu/Tiling-Assistant) adds practical
snap assist. Its [changelog](https://github.com/ubuntu/Tiling-Assistant/blob/main/CHANGELOG.md)
documents the GNOME 50 port. The actual pinned package's metadata is checked
against the GNOME major in the desktop-profile test.

Blur is deliberately absent. A [GNOME 50 memory-growth report](https://github.com/aunetx/blur-my-shell/issues/957)
and [dock interaction reports](https://github.com/aunetx/blur-my-shell/issues/615)
make that extra rendering layer unattractive for a small classroom VM and
remote-control sessions. These reports describe specific configurations and
versions; they do not prove all blur setups behave poorly.

## Existing accounts and validation

The login helper enables only the three required UUIDs, preserving other enabled
extensions. It migrates the specific appearance keys once and records
`~/.config/nixorium/desktop-style-v1`; later staff preferences survive. There
are no new dconf locks or complete-database resets. Student defaults return
with the ordinary home reset, rather than modifying a live student's files.

Run the optional `desktop-profile` check from `tests/source-checks.nix` for
strict GSettings compilation, extension compatibility and login-script syntax.
Keep that GNOME-dependent check separate from the lightweight Go gate. Build
one affected controller/client pair using existing cache, then verify login,
window snapping, icons and native Veyon sharing on the actual target before
claiming runtime or hardware validation.
