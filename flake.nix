{
  description = "Sticky notes desktop app (Wails v3)";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { nixpkgs, ... }:
    let
      systems = [ "x86_64-linux" "aarch64-linux" ];
      forAll = f: nixpkgs.lib.genAttrs systems (s: f nixpkgs.legacyPackages.${s});
    in {
      devShells = forAll (pkgs: {
        default = pkgs.mkShell {
          packages = with pkgs; [
            go
            nodejs
            pkg-config
            gtk4
            webkitgtk_6_0
            libsoup_3
            glib-networking
            gsettings-desktop-schemas
          ];
          shellHook = ''
            export PATH="$HOME/go/bin:$PATH"
            export GIO_MODULE_DIR=${pkgs.glib-networking}/lib/gio/modules/
            export XDG_DATA_DIRS=${pkgs.gsettings-desktop-schemas}/share/gsettings-schemas/${pkgs.gsettings-desktop-schemas.name}:${pkgs.gtk4}/share/gsettings-schemas/${pkgs.gtk4.name}:$XDG_DATA_DIRS
          '';
        };
      });
    };
}
