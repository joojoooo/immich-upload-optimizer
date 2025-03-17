#!/usr/bin/env bash
set -euxo pipefail

if [[ $# -lt 4 ]]; then
  echo "Usage: $0 <folder> <name> <extension> <result_folder>"
  exit 1
fi

ORIGINAL_FILE="${1}/${2}.${3}"
RESULT_FILE_NO_EXT="${4}/${2}"

convert ${ORIGINAL_FILE} ${ORIGINAL_FILE}.png
cjxl --lossless_jpeg=0 -q 75 ${ORIGINAL_FILE}.png ${RESULT_FILE_NO_EXT}.jxl
rm ${ORIGINAL_FILE}.png
#vips copy ${ORIGINAL_FILE} ${RESULT_FILE_NO_EXT}.jxl[Q=75]