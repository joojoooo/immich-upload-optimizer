# Immich Upload Optimizer
Immich Upload Optimizer (IOU) is a proxy designed to be placed in front of the [Immich](https://immich.app/) server. It intercepts file uploads and uses external CLI programs (by default: [AVIF](https://aomediacodec.github.io/av1-avif/), [JPEG-XL](https://jpegxl.info/), [FFmpeg](https://www.ffmpeg.org/)) to optimize, resize, or compress images and videos to save storage space

## ☕  Support the project
Love this project? You can [support it on ko-fi](https://ko-fi.com/svilex)! Every contribution helps keep the project alive!

[![ko-fi](https://www.ko-fi.com/img/githubbutton_sm.svg)](https://ko-fi.com/svilex)

## 🎯 About
This project is a fork of [miguelangel-nubla/immich-upload-optimizer](https://github.com/miguelangel-nubla/immich-upload-optimizer). It was created because the original author has different ideas and goals. This way I can add features faster, without having to convince or ask anyone

## ✨ Features
Features that differentiate this fork from the original project:

- **Longer disk lifespan**
  - Writes temporary files to RAM by default (tmpfs). Frequently writing to disk reduce its lifespan
  - Does less disk writes even with tmpfs disabled by not making useless copies of uploaded files
- **Lower RAM usage**
  - Does chunked uploads using io.Pipe: streaming small chunks from disk as they are sent. This prevents a copy in RAM of the whole file to be uploaded
- **Usable mobile app**
  - Doesn't show duplicate assets on the mobile app
  - Replaces checksums and file names, making the app oblivious to the different file being uploaded
  - The app won't try to upload the same files again because of checksum mismatch, even if you reinstall
- **AVIF support**
  - A more compatible open image format with similar quality/size to JXL
- **Automatic JXL->JPG conversion**
  - Automatically converts JXL to JPG on the fly when downloading images for better compatibility
- **Easier tasks config**
  - Default passthrough of any unprocessed image/video instead of having to add an empty task and list all extensions to allow
  - No need for a command to remove the original file, it's still needed if processing produces a bigger file size. IUO will delete it

## 🐋 Usage via Docker compose
Edit your Immich Docker Compose file:

```yaml
services:
  immich-upload-optimizer:
    image: ghcr.io/joojoooo/immich-upload-optimizer:latest
    tmpfs:
      - /tempfs
    ports:
      - "2284:2284"
    environment:
      - IUO_UPSTREAM=http://immich-server:2283
      - IUO_LISTEN=:2284
      - IUO_TASKS_FILE=/etc/immich-upload-optimizer/config/lossy_avif.yaml
      #- IUO_CHECKSUMS_FILE=/IUO/checksums.csv # Uncomment after defining a volume
      - TMPDIR=/tempfs # Writes uploaded files in RAM to improve disk lifespan (Remove if running low on RAM)
      #- IUO_DOWNLOAD_JPG_FROM_JXL=true # Uncomment to enable JXL conversion
    volumes:
      #- /path/to/your/host/dir:/IUO # Keep the checksums and tasks files between updates by defining a volume
    depends_on:
      - immich-server

  immich-server:
  # ...existing configuration...
  # remove the ports section if you only want to access immich through the proxy.
```
Run the appropriate commands at the `docker-compose.yml` location to stop, update and start the container:
```sh
# Stop container and edit docker-compose.yml
docker compose down
# Pull updates
docker compose pull
# Start container
docker compose up -d
```
Configure your **[tasks configuration file](TASKS.md)**

## 🚩 Flags
All flags are also available as environment variables using the prefix `IUO_` followed by the uppercase flag.
- `-upstream`: The URL of the Immich server (default: `http://immich-server:2283`)
- `-listen`: The address on which the proxy will listen (default: `:2284`)
- `-tasks_file`: Path to the [configuration file](TASKS.md) (default: [`lossy_avif.yaml`](config/lossy_avif.yaml))
- `-checksums_file`: Path to the checksums file (default: `checksums.csv`)
- `-download_jpg_from_jxl`: Converts JXL images to JPG on download for compatibility (default: `false`)

## 📸 Images
**[AVIF](https://aomediacodec.github.io/av1-avif/)** is used by default, saving **~80%** space while maintaining the same perceived quality (lossy conversion)
- It's an open format
- Offers good compatibility: it's easy to view or share the image with others
- Better than re-transcoding older formats (e.g., converting JPEG to a lower-quality JPEG)

**[JPEG-XL](https://jpegxl.info/)** is a superior format to AVIF, has all AVIF's pros and more, except it lacks widespread compatibility 😔
- Can losslessly convert JPEG to save **~20%** in space without losing any quality
- Support bit-accurate conversion back to the original JPEG
- A lossy JXL option is also available with similar quality/size ratio to AVIF

If neither fits your needs, create your own conversion task: examples in [config](config)

**To experiment with different quality settings live before modifying the task:** [squoosh.app](https://squoosh.app/), [caesium.app](https://caesium.app/)

> [!NOTE]
> Don't judge image compression artifacts by looking at the [Immich](https://github.com/immich-app/immich) low quality preview, zoom the image or download it and use an external viewer (Zooming on the Immich viewer will load the original image only if your browser is compatible with the format)

## 🎬 Videos
Lossy **[H.265](wikipedia.org/wiki/High_Efficiency_Video_Coding)** CRF23 60fps is used by default to ensure storage savings even for short videos while maintaining the same perceived quality.

All metadata is preserved and the video is not rotated (a different rotation than the original would cause viewing issues in the immich app)<br>
Lowering FPS or audio quality isn't worth it, would only give negligible file size savings for a much worse output<br>
It's recommended to only modify CRF and -preset speed to achieve the quality and speed you're after

## License
This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details

## Acknowledgements
- [miguelangel-nubla/immich-upload-optimizer](https://github.com/miguelangel-nubla/immich-upload-optimizer)
- [JamesCullum/multipart-upload-proxy](https://github.com/JamesCullum/multipart-upload-proxy)
- [Caesium](https://github.com/Lymphatus/caesium)
- [libjxl](https://github.com/libjxl/libjxl)
- [HandBrakeCLI](https://github.com/HandBrake/HandBrake)
- [Immich](https://github.com/immich-app/immich)
