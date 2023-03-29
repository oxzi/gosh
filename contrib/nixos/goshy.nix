{ config, lib, pkgs, ... }:

with lib;
let
  cfg = config.programs.goshy;

  goshy = pkgs.writers.writeBashBin "goshy" ''
    show_help() {
      cat <<EOF
    Usage: goshy [Options] FILE
    Options:
      -b --burn              Burn after reading.
      -h --help              Print this help message.
      -u --only--url         Print only URL as response.
      -p --period TIMEFRAME  Set a custom expiry period.
    EOF
    }

    while getopts p:period: flag
    do
      case "$\{flag\}" in
        p) PERIOD_IS_SET=1;;
        period) PERIOD_IS_SET=1;;
      esac
    done > /dev/null 2>&1
    
    if [ -z $PERIOD_IS_SET ]; then
      ${lib.optionalString (cfg.defaults.expiryPeriod != null) "PERIOD='${cfg.defaults.expiryPeriod}'"}
    fi

    ${lib.optionalString cfg.defaults.burnAfterReading "BURN='-F burn=1'"} \
    ${lib.optionalString cfg.defaults.printOnlyUrl "ONLYURL='?onlyURL'"} \

    while :; do
      case "$1" in
        -p|--period)
          PERIOD=$2
          shift
          ;;

        -b|--burn)
          BURN='-F burn=1'
          ;;

        -u|--only-url)
          ONLYURL='?onlyURL'
          ;;

        -h|--help)
          show_help
          exit 0
          ;;

        *)
          FILE=$1
          break
      esac

      shift
    done

    PERIOD="-F period=$PERIOD"

    if [[ -z "$1" ]]; then
      show_help
      exit 1
    fi

    ${pkgs.curl}/bin/curl ${cfg.instance}$ONLYURL -F "file=@$1" $PERIOD $BURN 
  '';

in {
  options = {
    programs.goshy = {
      enable = mkEnableOption "Upload files to an instance of gosh! Go Share";
      instance = mkOption {
        type = types.str;
      };
      defaults = {
        burnAfterReading = mkEnableOption "Enable burn after reading by default";
        expiryPeriod = mkOption {
          type = types.nullOr types.str;
          description = "Set a custom default expiry period, e.g., one minute";
          default = "3d";
        };
        printOnlyUrl = mkEnableOption "Enable only printing the resulting URL";
      };
    };
  };

  config = mkIf cfg.enable {
    environment.systemPackages = [ goshy ];
  };
}
