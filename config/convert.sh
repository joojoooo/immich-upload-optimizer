#!/usr/bin/env bash
set -euxo pipefail

if [[ $# -lt 5 ]]; then
  echo "Usage: $0 <folder> <name> <extension> <result_folder> <result_extension>"
  exit 1
fi

ORIGINAL_FILE="${1}/${2}.${3}"
PNG_FILE=${ORIGINAL_FILE}.png
RESULT_FILE="${4}/${2}.${5}"

# If you prefer using libvips:
# vips copy "${ORIGINAL_FILE}" "${RESULT_FILE}"[Q=75]
# Otherwise:

# Convert the original file to lossless PNG so it can be used as an input in any command we use next
# !! This removes video data from LivePhotos !!
convert "${ORIGINAL_FILE}" "${PNG_FILE}"

case "${5}" in
  jxl)
    cjxl --lossless_jpeg=0 -q 75 "${PNG_FILE}" "${RESULT_FILE}"
    ;;
  avif)
    avifenc -q 70 "${PNG_FILE}" "${RESULT_FILE}"
    ;;
  *)
    echo "Unsupported <result_extension>: ${5}"
    ;;
esac

rm "${PNG_FILE}"