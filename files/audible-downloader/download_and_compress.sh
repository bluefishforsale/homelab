#!/bin/bash

# Exit immediately if a command exits with a non-zero status
set -e
# Return the exit status of the last command in the pipeline that failed
set -o pipefail

# Directories to work from
BASEDIR='/data01/services/audible-downloader'
TARGET='/data01/complete/audiobooks'

# Run scripts
for script in "download.sh" "compress.sh"; do
      "${BASEDIR}/${script}"
done

# files and dirs are needed to be owned by media:media for plex to serve
# correction of permissions to ensure proper access
# Check if TARGET exists and is not empty before changing permissions
if [ -d "$TARGET" ] && [ "$(find "$TARGET" -mindepth 1 2>/dev/null)" ]; then
  echo "Updating permissions for directories in $TARGET"
  find "${TARGET}" -type d -exec chmod 755 {} \;

  echo "Updating permissions for files in $TARGET"
  find "${TARGET}" -type f -exec chmod 644 {} \;
else
  echo "Warning: $TARGET does not exist or is empty. Skipping permission updates."
fi
