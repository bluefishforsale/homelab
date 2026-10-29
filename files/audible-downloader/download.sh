#!/bin/bash

# Exit immediately if a command exits with a non-zero status
set -e
# Return the exit status of the last command in the pipeline that failed
set -o pipefail

TARGET='/data01/complete/audiobooks'

# create download dir if does not exist
echo "Checking download dir, create if not exist"
test -d "${TARGET}/aax" || mkdir  "${TARGET}/aax"

# One call for the whole library: per-ASIN calls re-authenticated and re-fetched
# the library for every book, ~8 minutes a night to skip files already on disk.
echo "Downloading new library items"
audible download --all --ignore-errors --ignore-podcasts --annotation --aax-fallback --no-confirm --chapter --pdf --cover --output-dir "${TARGET}/aax"
