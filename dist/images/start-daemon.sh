#!/usr/bin/env bash
set -euo pipefail

CNI_SOCK=/run/cni/hybridnet.sock

if [[ -e "$CNI_SOCK" ]]
then
    echo "previous socket exists, remove and continue"
	rm ${CNI_SOCK}
fi

/hybridnet/hybridnet-daemon --bind-socket=${CNI_SOCK} "$@"
