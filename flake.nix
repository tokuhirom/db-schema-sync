{
  description = "CLI tool for synchronizing PostgreSQL schemas from S3 using psqldef";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    let
      version = "0.0.14";

      # SHA256 hashes from goreleaser checksums.txt
      hashes = {
        "x86_64-linux" = "5c240028d0fbed37aaac959c060c765b7b6dfff9dd6df68cdc8ff54e8e345242";
        "aarch64-linux" = "8e1505b073c4c7e371a7437165b09d01904bd8deb3b2de363ee440933406bf2a";
        "x86_64-darwin" = "31e36492c2ece093d3090774bef3fdab4ec377201e64f6363940bbd0b7e0558c";
        "aarch64-darwin" = "63861f6724d29b95e1300323455c546b49491f27d55e6bcc9ee749f9aee36c8f";
      };

      # Map Nix system to goreleaser naming
      systemToGoreleaser = system: {
        "x86_64-linux" = "5c240028d0fbed37aaac959c060c765b7b6dfff9dd6df68cdc8ff54e8e345242";
        "aarch64-linux" = "8e1505b073c4c7e371a7437165b09d01904bd8deb3b2de363ee440933406bf2a";
        "x86_64-darwin" = "31e36492c2ece093d3090774bef3fdab4ec377201e64f6363940bbd0b7e0558c";
        "aarch64-darwin" = "63861f6724d29b95e1300323455c546b49491f27d55e6bcc9ee749f9aee36c8f";
      }.${system};

    in
    flake-utils.lib.eachSystem [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ] (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        goreleaserSystem = systemToGoreleaser system;
      in
      {
        packages = {
          default = self.packages.${system}.db-schema-sync;

          db-schema-sync = pkgs.stdenv.mkDerivation {
            pname = "db-schema-sync";
            inherit version;

            src = pkgs.fetchurl {
              url = "https://github.com/tokuhirom/db-schema-sync/releases/download/v${version}/db-schema-sync_${version}_${goreleaserSystem}.tar.gz";
              sha256 = hashes.${system};
            };

            sourceRoot = ".";

            installPhase = ''
              mkdir -p $out/bin
              cp db-schema-sync $out/bin/
              chmod +x $out/bin/db-schema-sync
            '';

            meta = with pkgs.lib; {
              description = "CLI tool for synchronizing PostgreSQL schemas from S3 using psqldef";
              homepage = "https://github.com/tokuhirom/db-schema-sync";
              license = licenses.mit;
              maintainers = [ ];
              mainProgram = "db-schema-sync";
              platforms = [ "x86_64-linux" "aarch64-linux" "x86_64-darwin" "aarch64-darwin" ];
            };
          };
        };

        # Development shell (builds from source for development)
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            gopls
            golangci-lint
          ];
        };
      }
    );
}
