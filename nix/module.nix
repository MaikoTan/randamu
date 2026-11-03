self:
{ config, lib, pkgs, ... }:

let
  cfg = config.services.randamu;
  jsonFormat = pkgs.formats.json { };
  effectiveConfig = if cfg.configFile != null then cfg.configFile else jsonFormat.generate "randamu-config.json" cfg.settings;
  underHome = path: lib.hasPrefix "/home/" (toString path);
in
{
  options.services.randamu = {
    enable = lib.mkEnableOption "the randamu periodic pixiv image fetcher";

    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.stdenv.hostPlatform.system}.randamu;
      description = "The package providing the randamu executable.";
    };

    user = lib.mkOption {
      type = lib.types.str;
      default = "randamu";
      description = "User that runs the service.";
    };

    group = lib.mkOption {
      type = lib.types.str;
      default = "randamu";
      description = "Group that runs the service.";
    };

    createUser = lib.mkOption {
      type = lib.types.bool;
      default = true;
      description = "Whether to create the user and group automatically.";
    };

    imagesDir = lib.mkOption {
      type = lib.types.path;
      default = "/var/lib/randamu/images";
      description = "Directory where images are downloaded.";
    };

    stateDir = lib.mkOption {
      type = lib.types.path;
      default = "/var/lib/randamu";
      description = "Directory for state and pagination cursors.";
    };

    configFile = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      description = "Use this JSON config file directly; when null, settings is generated into one.";
    };

    settings = lib.mkOption {
      type = jsonFormat.type;
      default = { };
      description = "randamu configuration, see examples/config.json.";
    };

    refreshTokenFile = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      example = "/run/secrets/pixiv-refresh-token";
      description = "File containing the pixiv refresh token, injected via systemd LoadCredential.";
    };

    environmentFile = lib.mkOption {
      type = lib.types.nullOr lib.types.path;
      default = null;
      description = "Optional environment file (for example PIXIV_REFRESH_TOKEN=...).";
    };

    proxy = lib.mkOption {
      type = lib.types.nullOr lib.types.str;
      default = null;
      example = "http://127.0.0.1:7890";
      description = "Proxy URL used for downloads.";
    };

    onCalendar = lib.mkOption {
      type = lib.types.str;
      default = "daily";
      description = "systemd timer OnCalendar expression.";
    };

    randomizedDelaySec = lib.mkOption {
      type = lib.types.str;
      default = "1h";
      description = "Randomized timer delay to avoid a fixed request time.";
    };

    extraArgs = lib.mkOption {
      type = lib.types.listOf lib.types.str;
      default = [ ];
      description = "Extra command line arguments passed to randamu.";
    };
  };

  config = lib.mkIf cfg.enable {
    assertions = [
      {
        assertion = cfg.refreshTokenFile != null || cfg.environmentFile != null;
        message = "services.randamu requires refreshTokenFile or environmentFile to provide the refresh token.";
      }
    ];

    users.users = lib.mkIf cfg.createUser {
      ${cfg.user} = {
        isSystemUser = true;
        group = cfg.group;
        home = toString cfg.stateDir;
      };
    };

    users.groups = lib.mkIf cfg.createUser {
      ${cfg.group} = { };
    };

    systemd.tmpfiles.rules = [
      "d ${cfg.stateDir} 0750 ${cfg.user} ${cfg.group} -"
      "d ${cfg.imagesDir} 0750 ${cfg.user} ${cfg.group} -"
    ];

    systemd.services.randamu = {
      description = "randamu pixiv image fetcher";
      after = [ "network-online.target" ];
      wants = [ "network-online.target" ];
      serviceConfig = {
        Type = "oneshot";
        User = cfg.user;
        Group = cfg.group;
        UMask = "0027";
        ExecStart = [ "${cfg.package}/bin/randamu --config ${effectiveConfig}" ] ++ cfg.extraArgs;
        Environment = [
          "RANDAMU_IMAGES_DIR=${cfg.imagesDir}"
          "RANDAMU_STATE_DIR=${cfg.stateDir}"
        ] ++ lib.optional (cfg.proxy != null) "RANDAMU_PROXY=${cfg.proxy}";
        EnvironmentFile = lib.optional (cfg.environmentFile != null) cfg.environmentFile;
        LoadCredential = lib.optional (cfg.refreshTokenFile != null)
          "pixiv_refresh_token:${cfg.refreshTokenFile}";

        Restart = "on-failure";
        RestartSec = "15min";

        ProtectSystem = "strict";
        ProtectHome = if underHome cfg.imagesDir || underHome cfg.stateDir then false else "read-only";
        PrivateTmp = true;
        NoNewPrivileges = true;
        PrivateDevices = true;
        ProtectKernelTunables = true;
        ProtectKernelModules = true;
        ProtectControlGroups = true;
        RestrictAddressFamilies = [ "AF_INET" "AF_INET6" "AF_UNIX" ];
        ReadWritePaths = [ cfg.imagesDir cfg.stateDir ];
      };
    };

    systemd.timers.randamu = {
      description = "randamu periodic fetch";
      wantedBy = [ "timers.target" ];
      timerConfig = {
        Unit = "randamu.service";
        OnCalendar = cfg.onCalendar;
        Persistent = true;
        RandomizedDelaySec = cfg.randomizedDelaySec;
        OnBootSec = "5min";
      };
    };
  };
}
