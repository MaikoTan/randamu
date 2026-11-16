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
        version = "1.7.3";

        # libwebp 版本，与 internal/webp 的 dlopen 绑定要保持 ABI 兼容。
        libwebpVersion = "1.6.0";

        # 每个发行平台的 zig target 与产物文件名。Linux 固定旧 glibc 基线
        # 保证任何现代发行版都能 dlopen；arm 用 gnueabi（soft float）以匹配
        # Go 的 linux/arm 运行时 ABI。
        webpTargets = {
          linux_amd64 = {
            zig = "x86_64-linux-gnu.2.17";
            soname = "libwebp_linux_amd64.so";
          };
          linux_arm64 = {
            zig = "aarch64-linux-gnu.2.17";
            soname = "libwebp_linux_arm64.so";
          };
          linux_arm = {
            zig = "arm-linux-gnueabi.2.17";
            soname = "libwebp_linux_arm.so";
          };
          linux_386 = {
            zig = "x86-linux-gnu.2.17";
            soname = "libwebp_linux_386.so";
          };
          darwin_amd64 = {
            zig = "x86_64-macos";
            soname = "libwebp_macos_amd64.dylib";
            # install_name 让加载器按 @rpath 解析同目录的 dylib。
            extraLd = "-Wl,-install_name,@rpath/libwebp.dylib";
          };
          darwin_arm64 = {
            zig = "aarch64-macos";
            soname = "libwebp_macos_arm64.dylib";
            extraLd = "-Wl,-install_name,@rpath/libwebp.dylib";
          };
          windows_amd64 = {
            zig = "x86_64-windows-gnu";
            soname = "libwebp_windows_amd64.dll";
          };
          windows_arm64 = {
            zig = "aarch64-windows-gnu";
            soname = "libwebp_windows_arm64.dll";
          };
          windows_386 = {
            zig = "x86-windows-gnu";
            soname = "libwebp_windows_386.dll";
          };
        };

        # 为每个 target 生成一条 build_one 调用，插进 buildPhase。
        webpBuildCalls =
          pkgs.lib.concatStringsSep "\n" (pkgs.lib.mapAttrsToList
            (name: t:
              ''build_one "${name}" "${t.zig}" "${t.soname}" "${t.extraLd or ""}"'')
            webpTargets);

        # 用 zig 交叉编译极致精简的 libwebp，strip + gzip 后放进源码树供
        # internal/webp 的 //go:embed 内嵌。
        #
        # 为什么不用 nixpkgs 的 libwebp：
        #   1. nixpkgs 的 libwebp 链接的是运行环境的 glibc，在别的 glibc 版本
        #      下 dlopen 会因 GLIBC_ABI_DT_* 符号缺失而失败；这里固定 glibc
        #      2.14 基线，任何现代发行版都能加载。
        #   2. Windows 系统没有 libwebp，只能随发行版自带。
        #   3. 只编我们调用的 5 个符号，内嵌体积从 2.5MB 的 WASM 移植降到
        #      200-400KB。
        webpLib = pkgs.stdenv.mkDerivation {
          name = "randamu-libwebp-${libwebpVersion}";
          # GitHub tarball 含一层 libwebp-<rev>/ 根目录，内置 fetchurl 不会自动
          # 剥离，解压到 $TMPDIR/src 并 --strip-components=1 手动剥掉（见下）。
          src = builtins.fetchurl {
            url = "https://github.com/webmproject/libwebp/archive/v${libwebpVersion}.tar.gz";
            sha256 = "0di1izy60bzlw43xdpggw6gnzd2mx1nn7m7g4cvyxbzgng155a4k";
          };

          # zig 既当编译器又当交叉 strip 工具（zig strip 支持 ELF/Mach-O/PE），
          # 不必拉完整的 llvmPackages.llvm。
          nativeBuildInputs = [ pkgs.cmake pkgs.zig pkgs.binutils ];
          # CMake 调用的是编译器路径，必须让它能执行 zig 包装脚本。
          strictDeps = true;
          dontConfigure = true;
          # zig 需要可写的缓存目录（默认 homeless-shelter 路径不可写）。
          ZIG_GLOBAL_CACHE_DIR = "$TMPDIR/zig-global-cache";
          ZIG_LOCAL_CACHE_DIR = "$TMPDIR/zig-local-cache";

          buildPhase = ''
            runHook preBuild
            mkdir -p "$out"

            # 每个 target：生成 zig 包装脚本 → cmake（静态）→ 编译 → 整体链接
            # 成单个 .so/.dylib/.dll → strip → gzip 到 $out/<name>.gz。
            #
            # 关掉 SIMD 保证最大可移植性；关掉 mux/demux、外部图像解码器与全部
            # 命令行工具，只留编解码。链接时只取 libwebp.a 与 libsharpyuv.a：
            # 前者提供所有导出符号，后者以静态方式并进来（否则会多出
            # libsharpyuv.so 这个外部依赖）。
            # fetchurl 给的是 .tar.gz，GitHub 归档内含一层 libwebp-<rev>/，
            # 先解压到 $TMPDIR/src 并剥掉根目录，后面所有 cmake 都在这里跑。
            mkdir -p "$TMPDIR/src"
            tar -xzf "$src" -C "$TMPDIR/src" --strip-components=1

            build_one() {
              local name="$1"; local zig="$2"; local soname="$3"; local extra="$4"

              cd "$TMPDIR/src"
              local cc="$TMPDIR/cc-$name.sh"
              cat > "$cc" <<EOF
            #!/bin/sh
            exec zig cc -target $zig "\$@"
            EOF
              chmod +x "$cc"

              rm -rf build obj
              cmake -S . -B build \
                -DCMAKE_BUILD_TYPE=Release \
                -DBUILD_SHARED_LIBS=OFF \
                -DCMAKE_C_COMPILER="$cc" \
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
                -DWITH_SIMD=0 \
                > /dev/null
              cmake --build build -j"$NIX_BUILD_CORES" > /dev/null

              mkdir -p obj
              ( cd obj && ar x ../build/libwebp.a && ar x ../build/libsharpyuv.a )

              zig cc -target "$zig" -shared -o "$soname" -O2 \
                $extra obj/*.o -lm -lc
              # zig 0.15 没有 `zig strip` 子命令；用 LLVM objcopy（zig 自带）剥离符号。
              zig objcopy --strip-all "$soname" "$soname.tmp" 2>/dev/null && mv "$soname.tmp" "$soname" || true
              gzip -9 -c "$soname" > "$out/$name.gz"
            }

            ${webpBuildCalls}

            runHook postBuild
          '';

          dontInstall = true;
          meta = {
            description = "libwebp shared libraries for randamu, all release targets";
            license = pkgs.lib.licenses.bsd3;
          };
        };

        randamu = (pkgs.buildGoModule {
          pname = "randamu";
          inherit version;
          src = ./.;
          # vendor/ 目录已提交进仓库（含 purego 等依赖），buildGoModule 直接用
          # 它构建，无需沙箱内跑 go mod vendor，也避免 vendorHash 缓存问题。
          vendorHash = null;
          ldflags = [ "-s" "-w" "-X main.version=${version}" ];
          env.CGO_ENABLED = "0";
          # WebP 编解码走运行时 dlopen 内嵌库（internal/webp/lib/*.gz 由上面
          # 的 webpLib 生成，经 postPatch 注入源码树）。purego 在 Linux 上
          # 通过 //go:cgo_import_dynamic 链接 libdl/libc，二进制仍是单文件。
          # 旧 gen2brain WASM 后端已移除，不再需要 nodynamic 标签。
          postPatch = ''
            # //go:embed 要求每个目标平台的库齐备，否则交叉编译报错。
            # lib/ 目录为空时不会被 git 跟踪，先建目录再拷入。
            mkdir -p internal/webp/lib
            cp ${webpLib}/*.gz internal/webp/lib/
          '';
          meta = {
            description = "Periodic image fetcher with idempotent downloads";
            homepage = "https://github.com/MaikoTan/randamu";
            license = pkgs.lib.licenses.mit;
            mainProgram = "randamu";
          };
          # 暴露给手动调试：nix build .#randamu.libwebp
          passthru.libwebp = webpLib;
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