{ config, lib, pkgs, ... }:

with lib;

let
  gosh = pkgs.buildGoModule {
    name = "gosh";

    src = lib.cleanSource ./.;

    # TODO: One has to configure this one.
    vendorSha256 = "0000000000000000000000000000000000000000000000000000";

    CGO_ENABLED = 0;
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

        User = "gosh";
        Group = "gosh";

        NoNewPrivileges = true;

        ProtectProc = "noaccess";
        ProcSubset = "pid";

        ProtectSystem = "full";
        ProtectHome = true;

        ReadOnlyPaths = "/";
        ReadWritePaths = "${cfg.dataDir}";
        InaccessiblePaths = "/boot /etc /mnt /root -/lost+found";
        NoExecPaths = "/";
        ExecPaths = "${gosh}/bin/goshd";

        PrivateTmp = true;
        PrivateDevices = true;
        PrivateIPC = true;

        ProtectHostname = true;
        ProtectClock = true;
        ProtectKernelTunables = true;
        ProtectKernelModules = true;
        ProtectKernelLogs = true;
        ProtectControlGroups = true;

        LockPersonality = true;
        MemoryDenyWriteExecute = true;
        RestrictRealtime = true;
        RestrictSUIDSGID = true;
        RemoveIPC = true;
      };
    };

    users.users.gosh = {
      group = "gosh";
      home = cfg.dataDir;
      createHome = true;
      uid = gosh-uid;
      isSystemUser = true;
    };

    users.groups.gosh.gid = gosh-uid;
  };
}
