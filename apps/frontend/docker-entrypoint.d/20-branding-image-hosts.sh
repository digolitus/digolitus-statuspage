#!/bin/sh
# Turns BRANDING_IMAGE_HOSTS (comma-separated host names, the same list the
# backend accepts for logo/favicon URLs) into the $branding_img_src variable
# used by the CSP img-src in nginx.conf. Written to /tmp because the root
# filesystem is read-only. Invalid entries stop the container.
set -eu

dir=/tmp/nginx-branding
mkdir -p "$dir"
sources=""
for host in $(printf '%s' "${BRANDING_IMAGE_HOSTS:-}" | tr ',' ' '); do
    host=$(printf '%s' "$host" | tr 'A-Z' 'a-z')
    case "$host" in
        *[!a-z0-9.-]* | .* | *. | *..* | "" | *.*.*.*.*.*.*.*.*.*)
            echo "BRANDING_IMAGE_HOSTS: invalid host name '$host' (use e.g. cdn.example.com)" >&2
            exit 1
            ;;
        *.*) sources="$sources https://$host" ;;
        *)
            echo "BRANDING_IMAGE_HOSTS: invalid host name '$host' (use e.g. cdn.example.com)" >&2
            exit 1
            ;;
    esac
done
printf 'map $host $branding_img_src {\n    default "%s";\n}\n' "${sources# }" > "$dir/csp.conf"
