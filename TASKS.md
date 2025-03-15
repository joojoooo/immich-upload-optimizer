# Configuration File

The YAML tasks config file defines a list of commands to execute on the uploaded file based on its extension.

## Usage

- The first task in the list with a matching extension runs the command on the uploaded file
- If no task with a matching extension is found, the original file is sent to immich
- The command create only 1 file inside {{.result_folder}} at the end of a successful conversion, this file will be uploaded to immich no matter its name or extension

## Example Task

Below is an example task entry:

```yaml
  - name: jpeg-xl
    command: cjxl --lossless_jpeg=1 {{.folder}}/{{.name}}.{{.extension}} {{.result_folder}}/{{.name}}-new.jxl
    extensions:
      - jpeg
      - jpg
```

This task processes `.jpeg` and `.jpg` files.

- `extensions`: Specifies file extensions to match.
- `command`: Defines the processing command.

### Placeholder Variables

To ensure proper file handling, use these placeholders in your commands:

- `{{.result_folder}}`: Where the processed file must be placed.
- `{{.folder}}`: Directory the original file is in.
- `{{.name}}`: Original file name without extension.
- `{{.extension}}`: Original file extension.

## Process Overview

When a file is uploaded, IUO:

1. Creates a temporary folder, e.g., `/tmp/processing-3398346076`.
2. Saves the file with a unique name, e.g., `file-2612480203.jpg`.
3. Executes the configured task command:

   ```sh
   cjxl --lossless_jpeg=1 {{.folder}}/{{.name}}.{{.extension}} {{.result_folder}}/{{.name}}-new.jxl
   ```

   This translates to:

   ```sh
   cjxl --lossless_jpeg=1 /tmp/file-2612480203.jpg /tmp/processing-3398346076/file-2612480203-new.jxl
   ```

4. If successful, IUO takes the processed file and uploads it to Immich.

## Docker Setup

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

## Additional Notes

- Ensure file extensions and commands are correctly specified.
- Tasks execute in the order they appear in the configuration file (from top to bottom).
- Long-running tasks (e.g., video transcoding) may exceed HTTP timeouts. IUO attempts to mitigate this by sending periodic HTTP redirects, but tasks will continue in the background even if the client disconnects. The processed file will still be uploaded to Immich regardless of client disconnection.

