{
  description = "Dev environment for kube-startup-cpu-boost (Go) Kubernetes operator";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-utils,
    }:
    flake-utils.lib.eachDefaultSystem (
      system:
      let
        pkgs = import nixpkgs { inherit system; };
        lib = pkgs.lib;

        kubePkg = pkgs.buildGoModule {
          pname = "kube-startup-cpu-boost";
          version = "0.1.0";
          src = ./.;

          vendorHash = "sha256-Y7fnOgNfB3VKeh0ahbXOzwMR1Z+uLsuo7RHQIBfLrQU=";

          env = {
            CGO_ENABLED = "0";
          };
          ldflags = [
            "-s"
            "-w"
          ];
        };

        dockerWrapper = pkgs.writeShellScriptBin "docker" "exec ${pkgs.podman}/bin/podman \"$@\"";

        baseTools = with pkgs; [
          go_1_24
          gopls
          golangci-lint
          delve
          git
          jq
          nixpkgs-fmt
          gnumake
          kubectl
          kind
          podman
          dockerWrapper
        ];

        extraTools = lib.optionals (lib.hasAttr "staticcheck" pkgs) [ pkgs.staticcheck ];
      in
      {
        devShells.default = pkgs.mkShell {
          name = "kube-startup-cpu-boost-devshell";
          buildInputs = baseTools ++ extraTools;
          shellHook = ''
            export CGO_ENABLED=0
            export GOFLAGS="-mod=readonly"
            export PODMAN_SHORTNAME_MODE=permissive

            # Setup Podman configuration
            mkdir -p ~/.config/containers
            if [ ! -f ~/.config/containers/policy.json ]; then
              echo '{"default": [{"type": "insecureAcceptAnything"}]}' > ~/.config/containers/policy.json
            fi

            # Install controller tools if needed
            if ! command -v controller-gen >/dev/null 2>&1; then
              go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.16.3
            fi
            if ! command -v setup-envtest >/dev/null 2>&1; then
              go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
            fi
            echo "Kubernetes operator dev environment ready."
            echo "Go: $(go version)"
            echo "Run: make build | make test | golangci-lint run | kind create cluster"
          '';
        };

        packages.kube-startup-cpu-boost = kubePkg;
        packages.default = kubePkg;

        formatter = pkgs.nixpkgs-fmt;
      }
    );
}
