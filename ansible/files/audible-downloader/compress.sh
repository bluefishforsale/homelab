#!/bin/bash

# Exit immediately if a command exits with a non-zero status
set -e
# Return the exit status of the last command in the pipeline that failed
set -o pipefail

BASEDIR='/data01/services/audible-downloader'
TARGET='/data01/complete/audiobooks'

cd "${BASEDIR}"
audible library export
find . -type f -name '*.aax*' | sed 's|^\./||' |  while read file ; do

    #--no-clobber \
${BASEDIR}/AAXtoMP3 \
    -l 3 \
    -A c424e208  \
    -t "${TARGET}" \
    -D '$artist - $title' \
    -e:mp3 \
    --chaptered \
    --chapter-naming-scheme '$title - $(printf %0${#chaptercount}d $chapternum) $chapter' \
    --use-audible-cli-data \
    --audible-cli-library-file "${BASEDIR}/library.tsv" \
    "${TARGET}/aax/${file}"
done
