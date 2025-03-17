# Immich Upload Optimizer
Immich Upload Optimizer (IOU) is a proxy designed to be placed in front of the [Immich](https://immich.app/) server. It intercepts file uploads and uses external CLI programs (by default: [JPEG-XL](https://jpegxl.info/) and [FFmpeg](https://www.ffmpeg.org/)) to optimize, resize, or compress images and videos to save storage space

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
      - IUO_TASKS_FILE=/etc/immich-upload-optimizer/config/lossless.yaml
      # Writes uploaded files in RAM to improve disk lifespan (Remove if running low on RAM)
      - TMPDIR=/tempfs
    depends_on:
      - immich-server

  immich-server:
  # ...existing configuration...
  # remove the ports section if you only want to access immich through the proxy.
```
Start Docker containers:
```sh
docker compose up -d
```
Configure your **[tasks configuration file](TASKS.md)**

## 🚩 Flags
All flags are also available as environment variables using the prefix `IUO_` followed by the uppercase flag.
- `-upstream`: The URL of the Immich server (default: `http://immich-server:2283`)
- `-listen`: The address on which the proxy will listen (default: `:2284`)
- `-tasks_file`: Path to the [tasks configuration file](TASKS.md) (default: [lossless.yaml](config/lossless.yaml))
- `-filter_path`: The path to filter file uploads (default: `/api/assets`)
- `-filter_form_key`: The form key to filter file uploads (default: `assetData`)
- `-download_jpg_from_jxl`: Converts JXL images to JPG on download for compatibility (default: `false`)

## 📸 Images
By default, Immich Upload Optimizer uses lossless **[JPEG-XL](https://jpegxl.info/)** for images, resulting in the same quality at a lower size. This allows for bit-accurate conversion back to the original JPEG, losing no information in the process.
> [!NOTE]
> Don't judge image compression artifacts by looking at the [Immich](https://github.com/immich-app/immich) low quality preview. Download the image and use an external viewer !

If you want to save more storage space, modify your tasks config to perform lossy compression. This can reduce file size considerably (around -80%) while maintaining the same perceived quality. Examples in [config](config/)<br>
**To experiment with different quality settings live before modifying the task:** [Squoosh.app](https://squoosh.app/), [Caesium.app](https://caesium.app/)

## 🎬 Videos
By default video conversion is disabled since no known lossless video transcoding will be smaller in size. However there is a lot of potential with [lossy conversion](config/lossy.yaml)

## License
This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details

## Acknowledgements
- [miguelangel-nubla/immich-upload-optimizer](https://github.com/miguelangel-nubla/immich-upload-optimizer)
- [JamesCullum/multipart-upload-proxy](https://github.com/JamesCullum/multipart-upload-proxy)
- [Caesium](https://github.com/Lymphatus/caesium)
- [libjxl](https://github.com/libjxl/libjxl)
- [HandBrakeCLI](https://github.com/HandBrake/HandBrake)
- [Immich](https://github.com/immich-app/immich)
