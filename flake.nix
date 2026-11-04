{
  description = "randamu: periodic pixiv image fetcher";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    (flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        version = "1.5.1";

        randamu = pkgs.buildGoModule {
          pname = "randamu";
          inherit version;
          src = ./.;
          # Standard library only; if third-party dependencies are added later,
          # replace this with the real hash reported by the Nix build.
          vendorHash = null;
          ldflags = [ "-s" "-w" "-X main.version=${version}" ];
          env.CGO_ENABLED = "0";
          meta = {
            description = "Periodic pixiv image fetcher with idempotent downloads";
            homepage = "https://github.com/MaikoTan/randamu";
            license = pkgs.lib.licenses.mit;
            mainProgram = "randamu";
          };
        };
      in
      {
        packages = {
          default = randamu;
          inherit randamu;
        };

        apps.default = flake-utils.lib.mkApp { drv = randamu; };

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gopls
            gotools
            golangci-lint
          ];
          shellHook = ''
            # Prevent Go from trying to download a toolchain in the sandbox.
            export GOTOOLCHAIN=local
          '';
        };

        formatter = pkgs.nixfmt-rfc-style;
      }))
    // {
      nixosModules.default = import ./nix/module.nix self;
      homeManagerModules.default = import ./nix/hm-module.nix self;
    };
}
