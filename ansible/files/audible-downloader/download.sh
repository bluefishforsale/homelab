#!/bin/bash

# Exit immediately if a command exits with a non-zero status
set -e
# Return the exit status of the last command in the pipeline that failed
set -o pipefail

TARGET='/data01/complete/audiobooks'

test -d  "${TARGET}/aax" || mkdir  "${TARGET}/aax"

ISNS=$(audible library  list | awk -F: '{print $1}')
for ISN in $ISNS ; do
    audible download -a "${ISN}" --annotation --aax-fallback --no-confirm --chapter --pdf --cover --output-dir "${TARGET}/aax"
done