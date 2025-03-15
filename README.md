# Immich Upload Optimizer

Immich Upload Optimizer is a proxy designed to be placed in front of the Immich server. It intercepts file uploads and uses an external CLI program (by default [JPEG-XL](https://github.com/libjxl/libjxl), [Caesium](https://github.com/Lymphatus/caesium-clt) and [HandBrake](https://github.com/HandBrake/HandBrake)) to optimize, resize, or compress images and videos before they are stored on the Immich server. This helps save storage space on the Immich server by reducing the size of uploaded files.

## About This Project

This project is a fork of the original idea by [miguelangel-nubla/immich-upload-optimizer](https://github.com/miguelangel-nubla/immich-upload-optimizer).<br>
It has been designed with the following key goals:

- **Lower RAM usage for file upload on RAM (tmpfs)**  
  Doesn't kill your disk
- **Better mobile app compatibility**  
  Doesn't show duplicate assets
- **Seamless JXL->JPG conversion**  
  Can automatically convert JXL to JPG on the fly when downloading images for better compatibility

## 🌟 Support the Project

Love this project? You can now [sponsor it on ko-fi](https://ko-fi.com/svilex) ! Every contribution helps keep the project growing and improving.

[![ko-fi](https://www.ko-fi.com/img/githubbutton_sm.svg)](https://ko-fi.com/svilex)

## Features

- Intercepts file uploads to the Immich server.
- You can use any external CLI program to optimize, resize, or compress files.
- Designed to be easily integrated into existing Immich installations using Docker Compose.

## Quality

By default, Immich Upload Optimizer uses lossless optimization for **images**, ensuring that no information is lost during the image optimization process. This means that the quality of your images remains unchanged while reducing their file size.

> [!NOTE]
> Image viewer in Immich will not show the stored image, so you can find compression artifacts.
> Download the file and open it with an external viewer to view the real image stored on your library.

If you prefer to save more storage space, you can modify the optimization parameters to perform lossy optimization. This can reduce the file size considerably (around 80% less) while maintaining the same perceived quality. To do this, adjust the task command to use a lossy compression setting. Examples in [config](config/).

### Images
You can use [Caesium.app](https://caesium.app/) to experiment with different quality settings live before modifying the task according to the optimizer documentation. For the specific parameters, refer to the [Caesium CLI documentation](https://github.com/Lymphatus/caesium-clt). Alternatively, use [Squoosh.app](https://squoosh.app/) to do the same thing for the [JPEG-XL](https://github.com/libjxl/libjxl) converter.

### Video
By default video conversion is disabled since no known lossless video transcoding will be smaller in size. However there is a lot of potential with lossy compression. HandBrake is included in the full image, take a look at how to do [lossy conversion](config/lossy.yaml).

## Usage via docker compose

1. Update your Docker Compose file:


```yaml
services:
  immich-upload-optimizer:
    image: ghcr.io/miguelangel-nubla/immich-upload-optimizer:latest
    tmpfs:
      - /tempfs
    ports:
      - "2284:2284"
    environment:
      - IUO_UPSTREAM=http://immich-server:2283
      - IUO_LISTEN=:2284
      - IUO_TASKS_FILE=/etc/immich-upload-optimizer/config/lossless.yaml
      - # Writes uploaded files in RAM to improve disk lifespan (Remove if running low on RAM)
      - TMPDIR=/tempfs
    depends_on:
      - immich-server

  immich-server:
  # ...existing configuration...
  # remove the ports section if you only want to access immich through the proxy.
```

2. Restart your Docker Compose services:

    ```sh
    docker compose restart
    ```

3. Configure your **[tasks configuration file](TASKS.md)**

## Available flags

  - `-upstream`: The URL of the Immich server (default: `http://immich-server:2283`).
  - `-listen`: The address on which the proxy will listen (default: `:2284`).
  - `-tasks_file`: Path to the [tasks configuration file](TASKS.md) (default: [lossless.yaml](config/lossless.yaml)).
  - `-filter_path`: The path to filter file uploads (default: `/api/assets`).
  - `-filter_form_key`: The form key to filter file uploads (default: `assetData`).
  - `-download_jpg_from_jxl`: Converts JXL images to JPG on download for wider compatibility (default: `false`).

  All flags are available as environment variables using the prefix `IUO_` followed by the uppercase flag.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.

## Acknowledgements

- [miguelangel-nubla/immich-upload-optimizer](https://github.com/miguelangel-nubla/immich-upload-optimizer) for the original idea.
- [JamesCullum/multipart-upload-proxy](https://github.com/JamesCullum/multipart-upload-proxy)
- [Caesium](https://github.com/Lymphatus/caesium)
- [libjxl](https://github.com/libjxl/libjxl)
- [HandBrakeCLI](https://github.com/HandBrake/HandBrake)
- [Immich](https://github.com/immich-app/immich)
