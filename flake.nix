{
  description = "Pogo — sticky notes that float above your windows (Wails v3)";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { nixpkgs, ... }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" ];
      forAll = f: nixpkgs.lib.genAttrs systems (s: f nixpkgs.legacyPackages.${s});
      shell = pkgs: gtk: webkit: pkgs.mkShell {
        packages = with pkgs; [
          go
          nodejs
          pkg-config
          gtk
          webkit
          libsoup_3
          glib-networking
          gsettings-desktop-schemas
        ];
        shellHook = ''
          export PATH="$HOME/go/bin:$PATH"
          export GIO_MODULE_DIR=${pkgs.glib-networking}/lib/gio/modules/
          export XDG_DATA_DIRS=${pkgs.gsettings-desktop-schemas}/share/gsettings-schemas/${pkgs.gsettings-desktop-schemas.name}:${gtk}/share/gsettings-schemas/${gtk.name}:$XDG_DATA_DIRS
        '';
      };
    in {
      devShells = forAll (pkgs: {
        # GTK 4 + WebKitGTK 6.0 (default; Ubuntu 24.04+, Fedora 39+, Arch)
        default = shell pkgs pkgs.gtk4 pkgs.webkitgtk_6_0;
        # GTK 3 + WebKit2GTK 4.1, built with `-tags gtk3` (Ubuntu 22.04, Debian 12)
        gtk3 = shell pkgs pkgs.gtk3 pkgs.webkitgtk_4_1;
      });
    };
}
