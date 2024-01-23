# Randamu

A simple python script to serve a webpage with random images, which can be used as wallpaper.

## Supported Services

- [x] [Pixiv](https://pixiv.net/) via [pixivpy_async](https://github.com/Mikubill/pixivpy-async)
    - You need to get your own refresh token from Pixiv, see [this guide](https://gist.github.com/ZipFile/c9ebedb224406f4f11845ab700124362) for details.
- [x] [Lolicon setu API](https://api.lolicon.app/#/setu)
- [x] [Stable Horde](https://stablehorde.net/)

## Features

- Press `N` or click the `➡️` emoji to switch to the next one without waiting.
- Press `L` or click the `💙` emoji to bookmark the current image
    - only available on Pixiv and Lolicon, requires Pixiv Refresh Token to be set.

## Usage

- **Make sure you have [Git](https://git-scm.com) and [Docker](https://docker.com) installed**

- Clone the repository

    ```bash
    git clone https://github.com/MaikoTan/randamu.git
    ```

- Run the docker container

    ```bash
    docker compose up -d
    ```

- Set your wallpaper

    Use `https://maikotan.github.io/randamu/` as the wallpaper webpage URL in your any supported wallpaper plugins.

    For example:

    - Linux (KDE): Use Plasma Wallpaper Plugin [HTML Wallpaper](https://store.kde.org/p/1324580)
    - Windows: [Lively Wallpaper](https://www.rocksdanister.com/lively/)
    - macOS: [Plash](https://sindresorhus.com/plash)

## Work in Progress

Since I am not familiar with Python, this project is not guaranteed to be well written.

## License

This Project is licensed under the [MIT License](LICENSE).
