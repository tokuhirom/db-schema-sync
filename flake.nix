{
  description = "CLI tool for synchronizing PostgreSQL schemas from S3 using psqldef";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    let
      version = "0.0.13";

      # SHA256 hashes from goreleaser checksums.txt
      hashes = {
        "x86_64-linux" = "062293585c23fe7b719cb04ee09bff72e8b7bbbfb1b48be9833da05375ade879";
        "aarch64-linux" = "e32c9e863299594ce868ac78f01f9ea071b830924b3f3cfc2573568e48e2115e";
        "x86_64-darwin" = "df768543389516d46e6a07237db1a3bf170d7b42b401a8ff287ddcd07d00ee86";
        "aarch64-darwin" = "5330469e0abf3220509f68da44778135d89b3ffdbb1a8f350274b328402697f1";
      };

      # Map Nix system to goreleaser naming
      systemToGoreleaser = system: {
        "x86_64-linux" = "linux_amd64";
        "aarch64-linux" = "linux_arm64";
        "x86_64-darwin" = "darwin_amd64";
        "aarch64-darwin" = "darwin_arm64";
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
