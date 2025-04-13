#!/usr/bin/env bash
set -euxo pipefail

if [[ $# -lt 5 ]]; then
  echo "Usage: $0 <folder> <name> <extension> <result_folder> <result_extension>"
  exit 1
fi

ORIGINAL_FILE="${1}/${2}.${3}"
RESULT_FILE="${4}/${2}.${5}"

# !! HEIC conversion removes video data from LivePhotos (if the photo and the video come together in a single file) !!

# libvips doesn't produce better images and it's somehow 2x slower than converting the image to PNG first and 5x slower than using ImageMagick to directly convert without PNG intermediate.
# Doesn't come preinstalled in the container anymore to save image size. To install run: "apt update && apt install libvips-tools"
#
# vips copy "${ORIGINAL_FILE}" "${RESULT_FILE}"[Q=75]

case "${5}" in
  jxl)
    # Convert the original file to lossless PNG so it can be used as an input in any command
    convert "${ORIGINAL_FILE}" "${ORIGINAL_FILE}.png"
    cjxl --lossless_jpeg=0 -q 75 -e 7 "${ORIGINAL_FILE}.png" "${RESULT_FILE}"
    rm "${ORIGINAL_FILE}.png"
    ;;
  avif)
    # ImageMagick supports HEIC to AVIF conversion which is 3x faster then converting to PNG and then using avifenc
    convert -quality 75 "${ORIGINAL_FILE}" "${RESULT_FILE}"
    ;;
  *)
    echo "Unsupported <result_extension>: ${5}"
    ;;
esac