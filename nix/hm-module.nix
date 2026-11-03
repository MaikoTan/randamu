self:
{ config, lib, pkgs, ... }:

let
  cfg = config.services.randamu;
  jsonFormat = pkgs.formats.json { };
  effectiveConfig = if cfg.configFile != null then cfg.configFile else jsonFormat.generate "randamu-config.json" cfg.settings;
in
{
  options.services.randamu = {
    enable = lib.mkEnableOption "the randamu periodic pixiv image fetcher (user systemd)";

    package = lib.mkOption {
      type = lib.types.package;
      default = self.packages.${pkgs.stdenv.hostPlatform.system}.randamu;
      description = "The package providing the randamu executable.";
    };

    imagesDir = lib.mkOption {
      type = lib.types.path;
      default = "${config.xdg.dataHome}/randamu/images";
      description = "Directory where images are downloaded.";
    };

    stateDir = lib.mkOption {
      type = lib.types.path;
      default = "${config.xdg.stateHome}/randamu";
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
      example = "config.sops.secrets.\"pixiv/refresh_token\".path";
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

    systemd.user.services.randamu = {
      Unit = {
        Description = "randamu pixiv image fetcher";
        After = [ "network-online.target" ];
      };
      Service = {
        Type = "oneshot";
        ExecStart = "${cfg.package}/bin/randamu --config ${effectiveConfig}" + lib.optionalString (cfg.extraArgs != [ ]) (" " + lib.concatStringsSep " " cfg.extraArgs);
        Environment = [
          "RANDAMU_IMAGES_DIR=${cfg.imagesDir}"
          "RANDAMU_STATE_DIR=${cfg.stateDir}"
        ] ++ lib.optional (cfg.proxy != null) "RANDAMU_PROXY=${cfg.proxy}";
        EnvironmentFile = lib.optional (cfg.environmentFile != null) cfg.environmentFile;
        LoadCredential = lib.optional (cfg.refreshTokenFile != null)
          "pixiv_refresh_token:${cfg.refreshTokenFile}";
        Restart = "on-failure";
        RestartSec = "15min";
      };
    };

    systemd.user.timers.randamu = {
      Unit = {
        Description = "randamu periodic fetch";
      };
      Timer = {
        Unit = "randamu.service";
        OnCalendar = cfg.onCalendar;
        Persistent = true;
        RandomizedDelaySec = cfg.randomizedDelaySec;
        OnBootSec = "5min";
      };
      Install.WantedBy = [ "timers.target" ];
    };
  };
}
