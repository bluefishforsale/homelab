#!/bin/bash

# Exit immediately if a command exits with a non-zero status
set -e
# Return the exit status of the last command in the pipeline that failed
set -o pipefail

BASEDIR='/data01/services/audible-downloader'
TARGET='/data01/complete/audiobooks'

cd "${TARGET}"
test -d "${TARGET}/mp3" || mkdir  "${TARGET}/mp3"
echo "Exporting audible library"
audible library export

find ./aax -type f -name '*.aax*' | sed 's|^\./||' |   while read file ; do
    echo "Compressing ${TARGET}/aax/${file}"
    ${BASEDIR}/AAXtoMP3 \
        -l 3 \
        --no-clobber \
        -A c424e208  \
        -t "${TARGET}/mp3" \
        -D '$artist - $title' \
        -e:mp3 \
        --chaptered \
        --chapter-naming-scheme '$title - $(printf %0${#chaptercount}d $chapternum) $chapter' \
        --use-audible-cli-data \
        --audible-cli-library-file "${BASEDIR}/library.tsv" \
        "${TARGET}/${file}"
done
