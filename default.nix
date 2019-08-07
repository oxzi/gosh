{ config, lib, pkgs, ... }:

with lib;

let
  gosh-id = 9001;

  gosh = pkgs.buildGoModule {
    name = "gosh";

    src = lib.cleanSource ./.;
    modSha256 = "0fnsd662p9v9ly9cy14asnv4gyx1xfnrn19abiyk3z098i4f0k7d";
  };

  cfg = config.services.gosh;
in {
  options.services.gosh = {
    enable = mkEnableOption "gosh, HTTP file server";

    contactMail = mkOption {
      type = types.str;
      description = "E-Mail address for abuses or the like.";
    };

    dataDir = mkOption {
      default = "/var/lib/gosh";
      type = types.path;
      description = "Directory for gosh's store.";
    };

    listenAddress = mkOption {
      default = ":8080";
      type = types.str;
      description = "Listen on a specific IP address and port.";
    };

    maxFilesize = mkOption {
      default = "10MiB";
      type = types.str;
      description = "Maximum file size for uploads";
    };

    maxLifetime = mkOption {
      default = "24h";
      example = "2m";
      type = types.str;
      description = "Maximum lifetime for uploads";
    };
  };

  # TODO: MimeMap

  config = {
    systemd.services.gosh = mkIf cfg.enable {
      description = "gosh! Go Share";

      after = [ "network.target" ];
      wantedBy = [ "multi-user.target" ];

      serviceConfig = {
        ExecStart = ''
          ${gosh}/bin/goshd \
            -contact "${cfg.contactMail}" \
            -listen ${cfg.listenAddress} \
            -max-filesize ${cfg.maxFilesize} \
            -max-lifetime ${cfg.maxLifetime} \
            -store ${cfg.dataDir}
        '';

        Type = "simple";

        User = "gosh";
        Group = "gosh";
      };
    };

    users.users.gosh = {
      group = "gosh";
      home = cfg.dataDir;
      createHome = true;
      uid = gosh-id;
    };

    users.groups.gosh.gid = gosh-id;
  };
}
