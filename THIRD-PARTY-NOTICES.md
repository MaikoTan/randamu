# Third-party notices

The MIT license in `LICENSE` covers randamu itself. The Windows x64 and x86
release archives additionally ship Tesseract OCR under `tesseract/`, so the
promo-text check works without a separate installation. That directory
contains the following components, all taken unmodified from the official
UB Mannheim builds (except for stripped debug symbols):

## tesseract/

| Component | License | Upstream |
|---|---|---|
| Tesseract OCR 5.x (`tesseract.exe`, `libtesseract-5.dll`) | Apache-2.0 | https://github.com/tesseract-ocr/tesseract |
| Leptonica (`libleptonica-6.dll`, `liblept-5.dll`) | BSD-2-Clause | https://github.com/DanBloomberg/leptonica |
| libarchive (`libarchive-13.dll`) | BSD-2-Clause | https://github.com/libarchive/libarchive |
| BLAKE2 (`libb2-1.dll`) | CC0-1.0 | https://github.com/BLAKE2/libb2 |
| bzip2 (`libbz2-1.dll`) | BSD-4-Clause-like | https://sourceware.org/bzip2/ |
| OpenSSL (`libcrypto-3-x64.dll`) | Apache-2.0 | https://www.openssl.org/ |
| libdeflate (`libdeflate.dll`) | MIT | https://github.com/ebiggers/libdeflate |
| Expat (`libexpat-1.dll`) | MIT | https://github.com/libexpat/libexpat |
| GCC runtime (`libgcc_s_*.dll`, `libstdc++-6.dll`) | GPL-3.0-or-later with GCC Runtime Library Exception | https://gcc.gnu.org/ |
| winpthreads (`libwinpthread-1.dll`) | MIT | https://www.mingw-w64.org/ |
| giflib (`libgif-7.dll`) | MIT | https://sourceforge.net/projects/giflib/ |
| GNU libiconv / gettext (`libiconv-2.dll`, `iconv.dll`, `libintl-8.dll`) | LGPL-2.1-or-later | https://www.gnu.org/software/libiconv/ |
| jbigkit (`libjbig-0.dll`, `libjbig-2.dll`) | GPL-2.0-or-later | https://www.cl.cam.ac.uk/~mgk25/jbigkit/ |
| libjpeg-turbo (`libjpeg-8.dll`) | BSD-3-Clause / IJG | https://libjpeg-turbo.org/ |
| LZ4 (`liblz4.dll`, `liblz4-1.dll`) | BSD-2-Clause | https://github.com/lz4/lz4 |
| XZ Utils (`liblzma-5.dll`) | 0BSD | https://tukaani.org/xz/ |
| OpenJPEG (`libopenjp2-7.dll`, `libopenjp2.dll`) | BSD-2-Clause | https://github.com/uclouvain/openjpeg |
| libpng (`libpng16-16.dll`) | libpng-2.0 | http://www.libpng.org/ |
| libwebp (`libwebp-7.dll`, `libwebpmux-3.dll`, `libsharpyuv-0.dll`) | BSD-3-Clause | https://chromium.googlesource.com/webm/libwebp |
| Zstandard (`libzstd.dll`, `libzstd-1.dll`) | BSD-3-Clause | https://github.com/facebook/zstd |
| zlib (`zlib1.dll`) | Zlib | https://zlib.net/ |
| Lerc (`libLerc.dll`) | Apache-2.0 | https://github.com/Esri/lerc |
| libtiff (`libtiff-5.dll`, `libtiff-6.dll`) | libtiff (BSD-like) | http://www.simplesystems.org/libtiff/ |
| libcurl (`libcurl-4.dll`) | curl | https://curl.se/ |
| libxml2 (`libxml2-2.dll`) | MIT | https://gitlab.gnome.org/GNOME/libxml2 |
| LZO (`liblzo2-2.dll`) | GPL-2.0-or-later | https://www.oberhumer.com/opensource/lzo/ |
| libidn2 (`libidn2-0.dll`) | LGPL-3.0-or-later | https://www.gnu.org/software/libidn/ |
| nghttp2 (`libnghttp2-14.dll`) | MIT | https://nghttp2.org/ |
| libunistring (`libunistring-2.dll`) | LGPL-3.0-or-later | https://www.gnu.org/software/libunistring/ |
| PCRE (`libpcre-1.dll`) | BSD-3-Clause | https://www.pcre.org/ |
| libffi (`libffi-6.dll`) | MIT | https://sourceware.org/libffi/ |

The full Apache-2.0 text that Tesseract is distributed under is copied to
`tesseract/LICENSE`. The remaining components are unmodified upstream
binaries; their source and complete license texts are available at the URLs
above.

The English OCR model `tesseract/tessdata/eng.traineddata` comes from the
main `tessdata` repository (Apache-2.0):
https://github.com/tesseract-ocr/tessdata
