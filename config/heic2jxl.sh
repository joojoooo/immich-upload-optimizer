#!/usr/bin/env bash
set -euxo pipefail

if [[ $# -lt 4 ]]; then
  echo "Usage: $0 <folder> <name> <extension> <result_folder>"
  exit 1
fi

ORIGINAL_FILE="${1}/${2}.${3}"
RESULT_FILE_NO_EXT="${4}/${2}"

if [[ $(exiftool -b -MotionPhoto "$ORIGINAL_FILE") -gt 0 ]]; then
  echo "Original file is a HEIC Motion Photo, video will be lost"
fi

vips copy ${ORIGINAL_FILE} ${RESULT_FILE_NO_EXT}-new.jxl[Q=75]