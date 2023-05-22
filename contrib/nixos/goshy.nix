{ config, lib, pkgs, ... }:

with lib;

let
  cfg = config.programs.goshy;

  goshy = pkgs.stdenv.mkDerivation rec {
    name = "goshy";

    src = ../../.;

    buildInputs = [ pkgs.bash pkgs.curl ];
    nativeBuildInputs = [ pkgs.makeWrapper ];

    installPhase = ''
      mkdir -p $out/bin
      cp ./contrib/bash/goshy $out/bin/goshy
      wrapProgram $out/bin/goshy \
        --set GOSH_INSTANCE "${cfg.instance}" \
        --set PERIOD "${cfg.expiryPeriod}" \
        ${lib.optionalString cfg.burnAfterReading "--set BURN 1"} \
        ${lib.optionalString cfg.printOnlyUrl "--set ONLYURL 1"}
    '';
  };
in
{
  options = {
    programs.goshy = {
      enable = mkEnableOption "Upload files to an instance of gosh! Go Share";

      instance = mkOption {
        type = types.str;
      };

      burnAfterReading = mkEnableOption "Enable burn after reading by default";

      expiryPeriod = mkOption {
        type = types.nullOr types.str;
        description = "Set a custom default expiry period, e.g., one minute";
        default = "3d";
      };

      printOnlyUrl = mkEnableOption "Enable only printing the resulting URL";
    };
  };

  config = mkIf cfg.enable {
    environment.systemPackages = [ goshy ];
  };
}
