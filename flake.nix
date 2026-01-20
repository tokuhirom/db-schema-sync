{
  description = "CLI tool for synchronizing PostgreSQL schemas from S3 using psqldef";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    let
      version = "0.0.15";

      # SHA256 hashes from goreleaser checksums.txt
      hashes = {
        "x86_64-linux" = "20fc1c784cd12340031354f7c5ee05ebe28e14ceb0c478daf5ec492ccb24cb12";
        "aarch64-linux" = "82f60b9e8508a83b560e0a575c0fa0e43923e1df16be77a07b47ccb17caee989";
        "x86_64-darwin" = "f45c9915c6ece5f696dffcd2bcd8fde25028f78745cbf031ad224045f2fb220b";
        "aarch64-darwin" = "beede7b06b8996ab81005fea2decfe5355f35ebf45031b8045a64e5fd39eb62b";
      };

      # Map Nix system to goreleaser naming
      systemToGoreleaser = system: {
        "x86_64-linux" = "20fc1c784cd12340031354f7c5ee05ebe28e14ceb0c478daf5ec492ccb24cb12";
        "aarch64-linux" = "82f60b9e8508a83b560e0a575c0fa0e43923e1df16be77a07b47ccb17caee989";
        "x86_64-darwin" = "f45c9915c6ece5f696dffcd2bcd8fde25028f78745cbf031ad224045f2fb220b";
        "aarch64-darwin" = "beede7b06b8996ab81005fea2decfe5355f35ebf45031b8045a64e5fd39eb62b";
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
