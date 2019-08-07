{ config, lib, pkgs, ... }:

with lib;

let
  gosh = pkgs.buildGoModule {
    name = "gosh";

    src = lib.cleanSource ./.;
    modSha256 = "0fnsd662p9v9ly9cy14asnv4gyx1xfnrn19abiyk3z098i4f0k7d";
  };

  gosh-uid = 9001;

  mimeMap = pkgs.writeText "goshd-mimemap" (
    map (x: "${x.from} ${x.to}") cfg.mimeMap);

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
      description = "Maximum file size for uploads.";
    };

    maxLifetime = mkOption {
      default = "24h";
      example = "2m";
      type = types.str;
      description = "Maximum lifetime for uploads.";
    };

    mimeMap = mkOption {
      default = [];
      example = [
        { from = "text/html"; to = "text/plain"; }
        { from = "image/gif"; to = "DROP"; }
      ];
      type = with types; listOf (submodule {
        options = {
          from = mkOption { type = str; };
          to   = mkOption { type = str; };
        };
      });
      description = "Map MIME types to others or DROP them.";
    };
  };

  config = mkIf cfg.enable {
    environment.systemPackages = [ gosh ];

    systemd.services.gosh = {
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
            -mimemap ${mimeMap} \
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
      uid = gosh-uid;
    };

    users.groups.gosh.gid = gosh-uid;
  };
}
