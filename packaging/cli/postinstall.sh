#!/bin/sh
# rfswift package post-install hook (deb postinst, rpm %post, pacman
# post_install/post_upgrade via nfpm). Do not invoke apt/dnf/pacman recursively
# while their transaction lock is held. Record pending interactive setup; the
# CLI offers it on its first terminal launch after the transaction completes.
mkdir -p /var/lib/rfswift 2>/dev/null || true
: > /var/lib/rfswift/host-setup-pending 2>/dev/null || true
if [ -t 1 ] || [ -n "${RFSWIFT_POSTINST_VERBOSE:-}" ]; then
  cat <<'MSG'

  RF Swift is installed. xhost and pactl were installed as package dependencies.
  The first interactive launch offers Nix and a Docker/Podman/Both/None choice.
  To start that setup immediately after this package command finishes:

      rfswift host setup

  It offers RF Swift's udev rules for SDR/RF hardware (reference copy in
  /usr/share/rfswift/udev/, needed by rootless Podman and Nix environments),
  installs Docker and/or Podman from your distribution if you want, and can
  grant your user Docker access that works without logging out.
  'rfswift doctor' shows the current state at any time.

MSG
fi
exit 0
