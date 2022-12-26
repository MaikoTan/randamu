# Random HTML Wallpaper

A simple python script to serve a website with random images.

## Supported Services

- [x] [Lolicon setu API](https://api.lolicon.app/#/setu)

## Usage

- Clone the repository

    ```bash
    git clone https://github.com/MaikoTan/random-html-wallpaper.git
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
    uvicorn main:app --reload --port 8089 --host 0.0.0.0
    ```
