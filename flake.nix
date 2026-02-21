{
  description = "cloud-comfort - LLM-powered Terraform management";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
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
        pkgs = import nixpkgs {
          inherit system;
          config.allowUnfreePredicate =
            pkg:
            builtins.elem (pkgs.lib.getName pkg) [
              "terraform"
            ];
        };
      in
      {
        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            # Frontend dependencies
            nodejs_20
            nodePackages.npm
            nodePackages.typescript

            # Backend dependencies
            go
            gotools
            gopls
            terraform

            awscli2
          ];

          shellHook = '''';
        };
      }
    );
}
