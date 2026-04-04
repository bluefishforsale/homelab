#!/bin/bash

# Find media files with TrueHD audio tracks
# Usage: ./find_truehd_files.sh [--csv]

MEDIA_DIRS=(
    "/data01/complete/tv"
    "/data01/complete/movies"
)

OUTPUT_FORMAT="text"

if [[ "${1:-}" == "--csv" ]]; then
    OUTPUT_FORMAT="csv"
    echo "filepath,size_gb,total_audio_tracks,truehd_tracks"
fi

check_file() {
    local file="$1"
    local codec_info
    
    codec_info=$(ffprobe -v error -select_streams a -show_entries stream=codec_name -of csv=p=0 "$file" 2>/dev/null)
    
    if echo "$codec_info" | grep -q "truehd"; then
        local size
        local size_gb
        local audio_tracks
        local truehd_tracks
        
        size=$(stat -c%s "$file" 2>/dev/null || stat -f%z "$file" 2>/dev/null)
        size_gb=$(awk "BEGIN {printf \"%.2f\", $size/1024/1024/1024}")
        audio_tracks=$(echo "$codec_info" | wc -l)
        truehd_tracks=$(echo "$codec_info" | grep -c "truehd")
        
        if [[ "$OUTPUT_FORMAT" == "csv" ]]; then
            echo "\"$file\",$size_gb,$audio_tracks,$truehd_tracks"
        else
            echo "$file"
            echo "  Size: ${size_gb}GB | Audio tracks: $audio_tracks | TrueHD tracks: $truehd_tracks"
        fi
    fi
}

export -f check_file
export OUTPUT_FORMAT

echo "Scanning for TrueHD audio tracks..." >&2

for dir in "${MEDIA_DIRS[@]}"; do
    if [[ ! -d "$dir" ]]; then
        echo "Warning: Directory not found: $dir" >&2
        continue
    fi
    
    echo "Scanning $dir..." >&2
    find "$dir" -type f \( -iname "*.mkv" -o -iname "*.mp4" \) -print0 2>/dev/null | \
        xargs -0 -P 8 -I {} bash -c 'check_file "$@"' _ {}
done

echo "Scan complete!" >&2
