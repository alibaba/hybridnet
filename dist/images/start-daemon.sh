#!/usr/bin/env bash
set -euo pipefail

CNI_SOCK=/run/cni/rama.sock

if [[ -e "$CNI_SOCK" ]]
then
    echo "previous socket exists, remove and continue"
	rm ${CNI_SOCK}
fi

/rama/rama-daemon --bind-socket=${CNI_SOCK} "$@"
