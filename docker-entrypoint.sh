#!/bin/sh
set -e

# ssh refuses keys and config files with loose permissions or a foreign
# owner, which bind mounts from the host usually have. Copy the mounted
# /ssh directory (private key, known_hosts, optional config) into place.
if [ -d /ssh ] && [ "$(ls -A /ssh 2>/dev/null)" ]; then
	mkdir -p /root/.ssh
	cp -R /ssh/. /root/.ssh/
	chown -R root:root /root/.ssh
	chmod 700 /root/.ssh
	find /root/.ssh -type f -exec chmod 600 {} +
	for f in /root/.ssh/*.pub /root/.ssh/known_hosts; do
		[ -f "$f" ] && chmod 644 "$f"
	done
fi

exec "$@"
