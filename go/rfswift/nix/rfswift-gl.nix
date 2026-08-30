# OpenGL / EGL runtime for hosts that are not NixOS.
#
# Programs from nixpkgs link against nixpkgs' libglvnd and Mesa, and those look
# for GPU drivers where NixOS installs them: /run/opengl-driver. On any other
# distribution that directory does not exist, so EGL and GLX never find a
# driver. GLFW then reports "EGL: Failed to get EGL display", every OpenGL
# version "was not supported", and SDR++ segfaults on the window it never got;
# Qt tools lose hardware acceleration or abort the same way.
#
# The fix, borrowed from nix-community/nixGL, is to point the loaders at Mesa's
# own drivers from this very nixpkgs pin through environment variables. This
# package ships them in two forms:
#
#   share/rfswift/gl.env   KEY=VALUE lines. The RF Swift nix engine reads them
#                          and merges them into every environment shell when the
#                          host is not NixOS (RFSWIFT_NIX_GL=off disables that).
#   bin/rfswift-gl         wrapper for manual use:  rfswift-gl sdrpp
#
# Mesa covers Intel, AMD and nouveau. The proprietary NVIDIA driver needs its
# own user-space libraries, matching the kernel module version, which cannot be
# known when the flake is evaluated. `rfswift-gl-nvidia` builds them impurely:
#
#   RFSWIFT_NVIDIA_VERSION=$(sed -n 's/.*Module  \([0-9.]*\)  .*/\1/p' /proc/driver/nvidia/version) \
#     nix build --impure .#pkg-rfswift-gl-nvidia
#
# The engine does this by itself, once per driver version. RFSWIFT_NVIDIA_HASH
# (SRI hash of the NVIDIA .run installer) makes that build reproducible.
# Both packages also ship bin/rfswift-gl-probe, which creates an OpenGL
# context the way GLFW/Qt tools do and prints the driver that answered
# (`rfswift nix gl --check` runs it), so a wrong runtime is diagnosed in one
# line instead of by a crashing SDR++.
#
# Driver matrix on Linux hosts that are not NixOS:
#   Intel (i915/xe), AMD (amdgpu/radeon), VMware, virtio, Raspberry Pi and
#   every other open driver: Mesa from this pin, through the host kernel.
#   NVIDIA proprietary: rfswift-gl-nvidia (this file with withNvidia), Mesa
#   kept behind it for hybrid laptops. NVIDIA on nouveau: Mesa.
# macOS needs nothing: nixpkgs programs use Apple's OpenGL/Metal directly.
{ lib, stdenv, runCommand, runtimeShell, mesa, libglvnd, linuxPackages, fetchurl, zstd
, withNvidia ? false, nvidiaVersion ? null, nvidiaHash ? null }:

let
  # libGLX_indirect is what libglvnd loads when the X server names no vendor
  # (indirect rendering, e.g. over ssh -X). Mesa's GLX serves it as well.
  glxindirect = runCommand "mesa-glxindirect" { } ''
    mkdir -p $out/lib
    ln -s ${mesa}/lib/libGLX_mesa.so.0 $out/lib/libGLX_indirect.so.0
  '';

  # A minimal EGL client: opens the display the way GLFW does (X11 when
  # DISPLAY is set, else Wayland, else Mesa's surfaceless platform), makes a
  # context current and prints GL_VENDOR / GL_RENDERER / GL_VERSION. Exit 1
  # with the EGL error when no context could be created - the same failure
  # SDR++ reports as "EGL: Failed to get EGL display".
  probe = stdenv.mkDerivation {
    pname = "rfswift-gl-probe";
    version = "1";
    dontUnpack = true;
    buildInputs = [ libglvnd ];
    buildPhase = ''
      cat > probe.c <<'C'
      #include <EGL/egl.h>
      #include <EGL/eglext.h>
      #include <GL/gl.h>
      #include <stdio.h>
      #include <stdlib.h>
      #include <string.h>

      #ifndef EGL_PLATFORM_SURFACELESS_MESA
      #define EGL_PLATFORM_SURFACELESS_MESA 0x31DD
      #endif

      static const char *egl_error(void) {
          static char buf[32];
          snprintf(buf, sizeof buf, "0x%04x", (unsigned)eglGetError());
          return buf;
      }

      static int try_platform(EGLenum platform, const char *name) {
          EGLDisplay dpy = eglGetPlatformDisplay(platform, EGL_DEFAULT_DISPLAY, NULL);
          if (dpy == EGL_NO_DISPLAY) {
              fprintf(stderr, "%s: no EGL display (%s)\n", name, egl_error());
              return 0;
          }
          EGLint major = 0, minor = 0;
          if (!eglInitialize(dpy, &major, &minor)) {
              fprintf(stderr, "%s: eglInitialize failed (%s)\n", name, egl_error());
              return 0;
          }
          if (!eglBindAPI(EGL_OPENGL_API)) {
              fprintf(stderr, "%s: no desktop OpenGL (%s)\n", name, egl_error());
              eglTerminate(dpy);
              return 0;
          }
          const EGLint attrs[] = {
              EGL_RENDERABLE_TYPE, EGL_OPENGL_BIT,
              EGL_SURFACE_TYPE, EGL_PBUFFER_BIT,
              EGL_RED_SIZE, 8, EGL_GREEN_SIZE, 8, EGL_BLUE_SIZE, 8,
              EGL_NONE };
          EGLConfig cfg;
          EGLint n = 0;
          EGLSurface surf = EGL_NO_SURFACE;
          if (eglChooseConfig(dpy, attrs, &cfg, 1, &n) && n > 0) {
              const EGLint pb[] = { EGL_WIDTH, 1, EGL_HEIGHT, 1, EGL_NONE };
              surf = eglCreatePbufferSurface(dpy, cfg, pb);
          } else {
              /* No pbuffer config: rely on EGL_KHR_surfaceless_context. */
              const EGLint any[] = { EGL_RENDERABLE_TYPE, EGL_OPENGL_BIT, EGL_NONE };
              if (!eglChooseConfig(dpy, any, &cfg, 1, &n) || n == 0) {
                  fprintf(stderr, "%s: no OpenGL config (%s)\n", name, egl_error());
                  eglTerminate(dpy);
                  return 0;
              }
          }
          EGLContext ctx = eglCreateContext(dpy, cfg, EGL_NO_CONTEXT, NULL);
          if (ctx == EGL_NO_CONTEXT) {
              fprintf(stderr, "%s: eglCreateContext failed (%s)\n", name, egl_error());
              eglTerminate(dpy);
              return 0;
          }
          if (!eglMakeCurrent(dpy, surf, surf, ctx)) {
              fprintf(stderr, "%s: eglMakeCurrent failed (%s)\n", name, egl_error());
              eglDestroyContext(dpy, ctx);
              eglTerminate(dpy);
              return 0;
          }
          const char *vendor = (const char *)glGetString(GL_VENDOR);
          const char *renderer = (const char *)glGetString(GL_RENDERER);
          const char *version = (const char *)glGetString(GL_VERSION);
          printf("platform: %s\nEGL: %d.%d %s\nvendor: %s\nrenderer: %s\nversion: %s\n",
                 name, major, minor, eglQueryString(dpy, EGL_VENDOR),
                 vendor ? vendor : "?", renderer ? renderer : "?", version ? version : "?");
          eglMakeCurrent(dpy, EGL_NO_SURFACE, EGL_NO_SURFACE, EGL_NO_CONTEXT);
          eglDestroyContext(dpy, ctx);
          if (surf != EGL_NO_SURFACE) eglDestroySurface(dpy, surf);
          eglTerminate(dpy);
          return 1;
      }

      int main(void) {
          const char *display = getenv("DISPLAY");
          const char *wayland = getenv("WAYLAND_DISPLAY");
          if (display && *display && try_platform(EGL_PLATFORM_X11_KHR, "x11")) return 0;
          if (wayland && *wayland && try_platform(EGL_PLATFORM_WAYLAND_KHR, "wayland")) return 0;
          if (try_platform(EGL_PLATFORM_SURFACELESS_MESA, "surfaceless")) return 0;
          fprintf(stderr, "no OpenGL context could be created\n");
          return 1;
      }
      C
      $CC -O2 -o rfswift-gl-probe probe.c -lEGL -lGL
    '';
    installPhase = ''
      install -Dm755 rfswift-gl-probe $out/bin/rfswift-gl-probe
    '';
    meta.platforms = lib.platforms.linux;
  };

  nvidiaArch = if stdenv.hostPlatform.isAarch64 then "aarch64" else "x86_64";
  nvidiaUrl = "https://download.nvidia.com/XFree86/Linux-${nvidiaArch}/${nvidiaVersion}/NVIDIA-Linux-${nvidiaArch}-${nvidiaVersion}.run";

  # nixpkgs' nvidia_x11 expression retargeted at the host's driver version and
  # reduced to the 64-bit user-space libraries: no kernel module (the host's
  # is what we match) and no 32-bit bundle. The license is the one the host
  # already accepted by running that driver.
  nvidiaLibs = (linuxPackages.nvidia_x11.override {
    libsOnly = true;
    disable32Bit = true;
    lib32 = null;
    acceptLicense = true;
  }).overrideAttrs (old: {
    pname = "nvidia";
    name = "nvidia-x11-${nvidiaVersion}-rfswift";
    version = nvidiaVersion;
    src =
      if nvidiaHash != null
      then fetchurl { url = nvidiaUrl; hash = nvidiaHash; }
      else builtins.fetchurl nvidiaUrl;
    nativeBuildInputs = (old.nativeBuildInputs or [ ]) ++ [ zstd ];
  });

  pname = if withNvidia then "rfswift-gl-nvidia" else "rfswift-gl";

  # Every variable is a path list; the engine and the wrapper keep whatever
  # the host already had behind these values.
  mesaEnv = ''
    GBM_BACKENDS_PATH=${mesa}/lib/gbm
    LIBGL_DRIVERS_PATH=${mesa}/lib/dri
    LIBVA_DRIVERS_PATH=${mesa}/lib/dri
    __EGL_VENDOR_LIBRARY_FILENAMES=${mesa}/share/glvnd/egl_vendor.d/50_mesa.json
    LD_LIBRARY_PATH=${mesa}/lib:${glxindirect}/lib:${libglvnd}/lib
  '';

  wrapper = ''
    #!${runtimeShell}
    # Run one program with the RF Swift GL runtime:  rfswift-gl sdrpp
    while IFS='=' read -r key value; do
      [ -n "$key" ] || continue
      current="''${!key:-}"
      export "$key=$value''${current:+:$current}"
    done < ${placeholder "out"}/share/rfswift/gl.env
    exec "$@"
  '';
in
if withNvidia && nvidiaVersion == null then
# Not an eval-time throw, so listing the flake's packages never fails: the
# build explains what is missing instead.
  runCommand pname { } ''
    echo "rfswift-gl-nvidia: set RFSWIFT_NVIDIA_VERSION to the host driver version (from /proc/driver/nvidia/version) and build with --impure" >&2
    exit 1
  ''
else
  runCommand pname
    {
      inherit mesaEnv wrapper;
      passAsFile = [ "mesaEnv" "wrapper" ];
      meta = with lib; {
        description = "OpenGL/EGL runtime for RF Swift Nix environments on non-NixOS hosts"
          + optionalString withNvidia " (NVIDIA ${nvidiaVersion})";
        platforms = platforms.linux;
        mainProgram = "rfswift-gl";
        license = if withNvidia then licenses.unfreeRedistributable else licenses.mit;
      };
    } ''
    mkdir -p $out/bin $out/share/rfswift
    env=$out/share/rfswift/gl.env
    # Fail this build, not the user's session, if the nixpkgs layout moves.
    test -d ${mesa}/lib/dri
    test -f ${mesa}/share/glvnd/egl_vendor.d/50_mesa.json
    test -e ${libglvnd}/lib/libEGL.so.1
    ${if withNvidia then ''
      # NVIDIA first, Mesa behind it: libglvnd tries EGL vendors in order and
      # asks the X server which GLX vendor drives the screen, so a hybrid
      # laptop (Intel/AMD display, NVIDIA offload) keeps working either way.
      # The vendor ICD file name varies between driver releases; glob it.
      icd=""
      for f in ${nvidiaLibs}/share/glvnd/egl_vendor.d/*nvidia*.json; do
        icd="$icd''${icd:+:}$f"
      done
      test -n "$icd"
      {
        echo "GBM_BACKENDS_PATH=${mesa}/lib/gbm"
        echo "LIBGL_DRIVERS_PATH=${mesa}/lib/dri"
        echo "LIBVA_DRIVERS_PATH=${mesa}/lib/dri"
        echo "__EGL_VENDOR_LIBRARY_FILENAMES=$icd:${mesa}/share/glvnd/egl_vendor.d/50_mesa.json"
        # Wayland and GBM windows on NVIDIA go through EGL "external
        # platforms" whose config files the driver expects under /usr/share.
        if [ -d ${nvidiaLibs}/share/egl/egl_external_platform.d ]; then
          echo "__EGL_EXTERNAL_PLATFORM_CONFIG_DIRS=${nvidiaLibs}/share/egl/egl_external_platform.d"
        fi
        echo "LD_LIBRARY_PATH=${libglvnd}/lib:${nvidiaLibs}/lib:${mesa}/lib:${glxindirect}/lib"
      } > "$env"
    '' else ''
      cp "$mesaEnvPath" "$env"
    ''}
    cp "$wrapperPath" $out/bin/rfswift-gl
    chmod +x $out/bin/rfswift-gl
    ln -s ${probe}/bin/rfswift-gl-probe $out/bin/rfswift-gl-probe
  ''
