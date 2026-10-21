#!/bin/sh
# Droidspaces mounts a fresh /tmp. procd needs its ubus socket directory before
# it can run the boot service that normally creates /var/run.
mkdir -p /tmp/run/ubus /tmp/lock /tmp/log
exec /sbin/procd
