# Randamu

A simple python script to serve a webpage with random images, which can be used as wallpaper.

## Supported Services

- [x] [Lolicon setu API](https://api.lolicon.app/#/setu)

## Usage

- Clone the repository

    ```bash
    git clone https://github.com/MaikoTan/randamu.git
    ```

- Create virtual environment

    ```bash
    python -m venv venv
    ```

- Activate the virtual environment

    ```bash
    # Linux
    source venv/bin/activate
    # Windows
    venv\Scripts\activate
    ```

- Install the requirements

    ```bash
    pip install -r requirements.txt
    ```

- Run the script

    ```bash
    uvicorn main:app --reload --port 8089
    # You could add `--host 0.0.0.0` to allow all IP addresses instead of `localhost`.
    ```

- Set your wallpaper

    Use `http://localhost:8089/` as the wallpaper webpage URL in your any supported wallpaper plugins.
    Parameters can be added as URL query like `http://localhost:8089/?tag=東方Project&r18=1`.

    For example:

    - Linux (KDE): Use Plasma Wallpaper Plugin [HTML Wallpaper](https://store.kde.org/p/1324580)
    - Windows: [Wallpaper Engine](https://www.wallpaperengine.io/)