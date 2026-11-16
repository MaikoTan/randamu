{
  description = "randamu: periodic pixiv image fetcher";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    (flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
        lib = pkgs.lib;
        version = "1.7.7";

        # libwebp 版本，与 internal/webp 的 cgo 绑定要保持 ABI 兼容。
        libwebpVersion = "1.6.0";

        # Linux 用 zig 交叉编译 musl 静态库并静态链接，产物在 glibc 与 musl
        # 发行版上都能直接运行；其它平台交给系统默认编译器（macOS 的
        # libSystem 本来就动态提供，无需 -static）。
        zigTarget = {
          x86_64-linux = "x86_64-linux-musl";
          aarch64-linux = "aarch64-linux-musl";
        }.${system} or null;

        # 把 libwebp 编成静态库（libwebp.a + libsharpyuv.a）并导出头文件，
        # 供 internal/webp 的 cgo 静态链接使用。
        #
        # 为什么自带而不是用 nixpkgs 的 libwebp：
        #   1. nixpkgs 的是 glibc 动态库，链接后二进制依赖 nix store 的 glibc，
        #      在非 NixOS 系统上无法直接运行；这里固定 musl 静态链接。
        #   2. 关掉 mux/demux、外部图像解码器与全部命令行工具，只留编解码，
        #      体积比完整 libwebp 小得多。
        libwebp = pkgs.stdenv.mkDerivation {
          name = "randamu-libwebp-static-${libwebpVersion}";
          src = builtins.fetchurl {
            url = "https://github.com/webmproject/libwebp/archive/v${libwebpVersion}.tar.gz";
            sha256 = "0di1izy60bzlw43xdpggw6gnzd2mx1nn7m7g4cvyxbzgng155a4k";
          };

          nativeBuildInputs = [ pkgs.cmake ] ++ lib.optional (zigTarget != null) pkgs.zig;
          strictDeps = true;
          dontConfigure = true;

          buildPhase = ''
            runHook preBuild
            # zig 需要可写的缓存目录（默认 homeless-shelter 路径不可写）。
            export ZIG_GLOBAL_CACHE_DIR="$TMPDIR/zig-global-cache"
            export ZIG_LOCAL_CACHE_DIR="$TMPDIR/zig-local-cache"
            mkdir -p "$TMPDIR/src"
            tar -xzf "$src" -C "$TMPDIR/src" --strip-components=1

            # CMake 的 CMAKE_C_COMPILER 必须是一个可执行路径，用包装脚本
            # 把 zig 的 -target 参数带进去。
            cat > "$TMPDIR/cc.sh" <<EOF
            #!/bin/sh
            exec ${if zigTarget == null then "cc" else "zig cc -target ${zigTarget}"} "\$@"
            EOF
            chmod +x "$TMPDIR/cc.sh"

            cmake -S "$TMPDIR/src" -B build \
              -DCMAKE_BUILD_TYPE=Release \
              -DBUILD_SHARED_LIBS=OFF \
              -DCMAKE_C_COMPILER="$TMPDIR/cc.sh" \
              -DCMAKE_C_FLAGS="-ffunction-sections -fdata-sections" \
              -DWEBP_BUILD_ANIM_UTILS=OFF \
              -DWEBP_BUILD_CWEBP=OFF \
              -DWEBP_BUILD_DWEBP=OFF \
              -DWEBP_BUILD_GIF2WEBP=OFF \
              -DWEBP_BUILD_IMG2WEBP=OFF \
              -DWEBP_BUILD_VWEBP=OFF \
              -DWEBP_BUILD_WEBPINFO=OFF \
              -DWEBP_BUILD_LIBWEBPMUX=OFF \
              -DWEBP_BUILD_EXTRAS=OFF \
              -DWITH_JPEG=OFF \
              -DWITH_PNG=OFF \
              -DWITH_TIFF=OFF \
              -DWITH_SIMD=0 > /dev/null
            cmake --build build -j"$NIX_BUILD_CORES" > /dev/null

            mkdir -p "$out/lib" "$out/include"
            cp build/libwebp.a build/libsharpyuv.a "$out/lib/"
            cp -r "$TMPDIR/src/src/webp" "$out/include/"
            runHook postBuild
          '';

          dontInstall = true;
          meta = {
            description = "Static libwebp (encode/decode only) for randamu";
            license = pkgs.lib.licenses.bsd3;
          };
        };

        # Go 的 cgo 需要一个可执行路径作 CC，同样用包装脚本注入 zig 目标。
        ccWrapper = pkgs.writeShellScriptBin "randamu-cc" ''
          exec ${if zigTarget == null then "cc" else "zig cc -target ${zigTarget}"} "$@"
        '';

        randamu = (pkgs.buildGoModule {
          pname = "randamu";
          inherit version;
          src = ./.;
          # 已无第三方 Go 依赖（WebP 改走 cgo），无需 vendor。
          vendorHash = null;

          # ccWrapper 在构建期需要 zig 本体。
          nativeBuildInputs = lib.optional (zigTarget != null) pkgs.zig;

          env = {
            CGO_ENABLED = "1";
            CGO_CFLAGS = "-I${libwebp}/include";
            CGO_LDFLAGS = "-L${libwebp}/lib -lwebp -lsharpyuv -lm";
          };
          # stdenv 的 setup hook 会把 CC 覆盖成 gcc，必须在 buildPhase 前
          # 重新导出，否则静态链接找不到 musl 的 libc。zig 的缓存目录默认
          # 在不可写的 homeless-shelter 下，也在这里重定向。
          preBuild = ''
            export CC="${ccWrapper}/bin/randamu-cc"
            export ZIG_GLOBAL_CACHE_DIR="$TMPDIR/zig-global-cache"
            export ZIG_LOCAL_CACHE_DIR="$TMPDIR/zig-local-cache"
          '';
          # 保持纯 Go 的 DNS/用户解析，避免 cgo 版在静态 musl 下引入 nss 依赖。
          tags = [ "netgo" "osusergo" ];
          ldflags = [ "-s" "-w" "-X main.version=${version}" ]
            ++ lib.optional (zigTarget != null) "-extldflags=-static";

          meta = {
            description = "Periodic image fetcher with idempotent downloads";
            homepage = "https://github.com/MaikoTan/randamu";
            license = pkgs.lib.licenses.mit;
            mainProgram = "randamu";
          };
          # 暴露给手动调试：nix build .#randamu.libwebp
          passthru.libwebp = libwebp;
        });
      in
      {
        packages = {
          default = randamu;
          inherit randamu;
        };

        apps.default = flake-utils.lib.mkApp { drv = randamu; };

        devShells.default = pkgs.mkShell {
          packages = with pkgs; [
            go
            gopls
            gotools
            golangci-lint
            # 本地 go build / go test 需要 libwebp 的头文件与库（动态链接即可）。
            libwebp
            pkg-config
          ];
          shellHook = ''
            # Prevent Go from trying to download a toolchain in the sandbox.
            export GOTOOLCHAIN=local
          '';
        };

        formatter = pkgs.nixfmt-rfc-style;
      }))
    // {
      nixosModules.default = import ./nix/module.nix self;
      homeManagerModules.default = import ./nix/hm-module.nix self;
    };
}
