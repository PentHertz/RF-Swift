#!/bin/sh
# rfswift package post-install hook (deb postinst, rpm %post, pacman
# post_install/post_upgrade via nfpm). It changes NOTHING on the host: the
# udev rules, the container engine and Docker socket access are deliberately
# left to the user, who is pointed at the command that asks before doing
# them. Never fails the package installation.
if [ -t 1 ] || [ -n "${RFSWIFT_POSTINST_VERBOSE:-}" ]; then
  cat <<'MSG'

  RF Swift is installed. Optional host setup (asks before every step):

      rfswift host setup

  It offers RF Swift's udev rules for SDR/RF hardware (reference copy in
  /usr/share/rfswift/udev/, needed by rootless Podman and Nix environments),
  installs Docker and/or Podman from your distribution if you want, and can
  grant your user Docker access that works without logging out.
  'rfswift doctor' shows the current state at any time.

MSG
fi
exit 0
