# Configuration File
The YAML tasks config file defines a list of commands to execute on the uploaded file based on its extension. [Examples here](config/)

## Commands
The Docker container comes with some popular commands preinstalled which can be used to create custom tasks:<br>
**Image:** [`cjxl`](https://github.com/libjxl/libjxl), [`caesiumclt`](https://github.com/Lymphatus/caesium-clt), [`magick`](https://imagemagick.org/script/command-line-tools.php)<br>
**Video:** [`ffmpeg`](https://www.ffmpeg.org)

## Usage
- The first task in the list with a matching extension runs the command on the uploaded file
- If no task with a matching extension is found, the original file is sent to immich
- The command must create only 1 file inside {{.result_folder}} at the end of a successful conversion, this file will be uploaded to immich no matter its name or extension

## Example Task
```yaml
  - name: jpeg-xl
    command: cjxl --lossless_jpeg=1 {{.folder}}/{{.name}}.{{.extension}} {{.result_folder}}/{{.name}}.jxl
    extensions:
      - jpeg
      - jpg
```
- `name`: Defines the task name that appears in logs
- `command`: Defines the processing command
- `extensions`: Specifies what file extensions this command will process

#### Placeholder Variables
- `{{.result_folder}}`: Where the processed file must be placed
- `{{.folder}}`: Directory the original file is in
- `{{.name}}`: Original file name without extension
- `{{.extension}}`: Original file extension

## Process Overview
When a file is uploaded, IUO:
- Saves the file with a unique name: `/tmp/upload-2612480203.jpg` = `{{.folder}}/{{.name}}.{{.extension}}`
- Creates a temporary folder: `/tmp/processing-3398346076` = `{{.result_folder}}`
- Executes the task command matching the file extension:
```sh
# (with placeholders replaced)
cjxl --lossless_jpeg=1 /tmp/upload-2612480203.jpg /tmp/processing-3398346076/upload-2612480203.jxl
```
- If successful and 1 file is found in the processing folder, IUO uploads it to Immich

## Additional Notes
- The processing command **must not modify** the original file
- Long-running tasks (e.g. video transcoding) may exceed HTTP timeouts. Tasks will continue in the background even if the client disconnects. The processed file will still be uploaded to Immich regardless of client disconnection. A WebSocket is also used to notify upload success so this shouldn't really matter (web portal currently ignores those notifications)
- Only 1 task per upload executes. If multiple tasks have the same extension, the one closer to the top of the config file executes